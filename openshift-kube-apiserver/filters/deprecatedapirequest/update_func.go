package deprecatedapirequest

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	apiv1 "github.com/openshift/api/apiserver/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/openshift-kube-apiserver/filters/deprecatedapirequest/v1helpers"
)

// IncrementRequestCounts add additional api request counts to the log.
// countsToPersist must not be mutated
func SetRequestCountsForNode(nodeName string, expiredHour int, countsToPersist *resourceRequestCounts) v1helpers.UpdateStatusFunc {
	return func(status *apiv1.APIRequestCountStatus) {
		existingLogsFromAPI := apiStatusToRequestCount(countsToPersist.resource, status)
		existingNodeLogFromAPI := existingLogsFromAPI.Node(nodeName)
		existingNodeLogFromAPI.ExpireOldestCounts(expiredHour)

		// updatedCounts is an alias so we recognize this, but it is based on the newly computed struct so we don't destroy
		// our input data.
		updatedCounts := existingNodeLogFromAPI.Resource(countsToPersist.resource)
		updatedCounts.Add(countsToPersist)
		hourlyRequestLogs := resourceRequestCountToHourlyNodeRequestLog(nodeName, updatedCounts)

		newStatus := setRequestCountsForNode(status, nodeName, expiredHour, hourlyRequestLogs)
		status.Last24h = newStatus.Last24h
		status.CurrentHour = newStatus.CurrentHour
		status.RemovedInRelease = removedRelease(countsToPersist.resource)
		status.RequestCount = newStatus.RequestCount

		// TODO remove when we start writing, but I want data tonight.
		content, _ := json.MarshalIndent(status.CurrentHour, "", "  ")
		klog.V(2).Infof("updating top %v APIRequest counts with last hour:\n%v", countsToPersist.resource, string(content))
	}
}

func setRequestCountsForNode(status *apiv1.APIRequestCountStatus, nodeName string, expiredHour int, hourlyNodeRequests []apiv1.PerNodeAPIRequestLog) *apiv1.APIRequestCountStatus {
	newStatus := status.DeepCopy()
	newStatus.Last24h = []apiv1.PerResourceAPIRequestLog{}
	newStatus.CurrentHour = apiv1.PerResourceAPIRequestLog{}

	for hour, currentNodeCount := range hourlyNodeRequests {
		totalRequestThisHour := int64(0)
		nextHourStatus := apiv1.PerResourceAPIRequestLog{}
		if hour == expiredHour {
			newStatus.Last24h = append(newStatus.Last24h, nextHourStatus)
			continue
		}
		if len(status.Last24h) > hour {
			for _, oldNodeStatus := range status.Last24h[hour].ByNode {
				if oldNodeStatus.NodeName == nodeName {
					continue
				}
				totalRequestThisHour += oldNodeStatus.RequestCount
				nextHourStatus.ByNode = append(nextHourStatus.ByNode, *oldNodeStatus.DeepCopy())
			}
		}
		nextHourStatus.ByNode = append(nextHourStatus.ByNode, currentNodeCount)
		totalRequestThisHour += currentNodeCount.RequestCount
		nextHourStatus.RequestCount = totalRequestThisHour

		newStatus.Last24h = append(newStatus.Last24h, nextHourStatus)
	}

	totalRequestsThisDay := int64(0)
	for _, hourCount := range newStatus.Last24h {
		totalRequestsThisDay += hourCount.RequestCount
	}
	newStatus.RequestCount = totalRequestsThisDay

	// get all our sorting before copying
	canonicalizeStatus(newStatus)
	currentHour := time.Now().Hour()
	newStatus.CurrentHour = newStatus.Last24h[currentHour]

	return newStatus
}

const numberOfUsersInAPI = 10

// in this function we have exclusive access to resourceRequestCounts, so do the easy map navigation
func resourceRequestCountToHourlyNodeRequestLog(nodeName string, resourceRequestCounts *resourceRequestCounts) []apiv1.PerNodeAPIRequestLog {
	hourlyNodeRequests := []apiv1.PerNodeAPIRequestLog{}
	for i := 0; i < 24; i++ {
		hourlyNodeRequests = append(hourlyNodeRequests,
			apiv1.PerNodeAPIRequestLog{
				NodeName: nodeName,
				ByUser:   nil,
			},
		)
	}

	for hour, hourlyCount := range resourceRequestCounts.hourToRequestCount {
		totalRequestsThisHour := int64(0)
		for user, userCount := range hourlyCount.usersToRequestCounts {
			apiUserStatus := apiv1.PerUserAPIRequestCount{
				UserName:     user,
				RequestCount: 0,
				ByVerb:       nil,
			}
			totalCount := int64(0)
			for verb, verbCount := range userCount.verbsToRequestCounts {
				totalCount += verbCount.count
				apiUserStatus.ByVerb = append(apiUserStatus.ByVerb,
					apiv1.PerVerbAPIRequestCount{
						Verb:         verb,
						RequestCount: verbCount.count,
					})
			}
			apiUserStatus.RequestCount = totalCount
			totalRequestsThisHour += totalCount

			// the api resource has an interesting property of only keeping the last few.  Having a short list makes the sort faster
			hasMaxEntries := len(hourlyNodeRequests[hour].ByUser) >= numberOfUsersInAPI
			if hasMaxEntries {
				currentSmallestCount := hourlyNodeRequests[hour].ByUser[len(hourlyNodeRequests[hour].ByUser)-1].RequestCount
				if apiUserStatus.RequestCount <= currentSmallestCount {
					continue
				}
			}

			hourlyNodeRequests[hour].ByUser = append(hourlyNodeRequests[hour].ByUser, apiUserStatus)
			sort.Stable(sort.Reverse(byNumberOfUserRequests(hourlyNodeRequests[hour].ByUser)))
		}
		hourlyNodeRequests[hour].RequestCount = totalRequestsThisHour
	}

	return hourlyNodeRequests
}

func apiStatusToRequestCount(resource schema.GroupVersionResource, status *apiv1.APIRequestCountStatus) *clusterRequestCounts {
	requestCount := newClusterRequestCounts()
	for hour, hourlyCount := range status.Last24h {
		for _, hourlyNodeCount := range hourlyCount.ByNode {
			for _, hourNodeUserCount := range hourlyNodeCount.ByUser {
				for _, hourlyNodeUserVerbCount := range hourNodeUserCount.ByVerb {
					requestCount.IncrementRequestCount(
						hourlyNodeCount.NodeName,
						resource,
						hour,
						hourNodeUserCount.UserName,
						hourlyNodeUserVerbCount.Verb,
						hourlyNodeUserVerbCount.RequestCount,
					)
				}
			}
		}
	}
	return requestCount
}

func canonicalizeStatus(status *apiv1.APIRequestCountStatus) {
	for hour := range status.Last24h {
		hourlyCount := status.Last24h[hour]
		for j := range hourlyCount.ByNode {
			nodeCount := hourlyCount.ByNode[j]
			for k := range nodeCount.ByUser {
				userCount := nodeCount.ByUser[k]
				sort.Stable(byVerb(userCount.ByVerb))
			}
			sort.Stable(sort.Reverse(byNumberOfUserRequests(nodeCount.ByUser)))
		}
		sort.Stable(byNode(status.Last24h[hour].ByNode))
	}

}

type byVerb []apiv1.PerVerbAPIRequestCount

func (s byVerb) Len() int {
	return len(s)
}
func (s byVerb) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s byVerb) Less(i, j int) bool {
	return strings.Compare(s[i].Verb, s[j].Verb) < 0
}

type byNode []apiv1.PerNodeAPIRequestLog

func (s byNode) Len() int {
	return len(s)
}
func (s byNode) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s byNode) Less(i, j int) bool {
	return strings.Compare(s[i].NodeName, s[j].NodeName) < 0
}

type byNumberOfUserRequests []apiv1.PerUserAPIRequestCount

func (s byNumberOfUserRequests) Len() int {
	return len(s)
}
func (s byNumberOfUserRequests) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s byNumberOfUserRequests) Less(i, j int) bool {
	return s[i].RequestCount < s[j].RequestCount
}
