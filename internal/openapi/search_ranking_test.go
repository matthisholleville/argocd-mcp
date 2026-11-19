package openapi

import (
	"os"
	"slices"
	"strings"
	"testing"
)

var retrievalCases = []struct {
	query string
	want  []string
}{
	{"list applications", []string{"ApplicationService_List"}},
	{"get application", []string{"ApplicationService_Get"}},
	{"refresh application", []string{"ApplicationService_Get"}},
	{"hard refresh app", []string{"ApplicationService_Get"}},
	{"sync application", []string{"ApplicationService_Sync"}},
	{"sync app", []string{"ApplicationService_Sync"}},
	{"rollback application", []string{"ApplicationService_Rollback"}},
	{"delete application", []string{"ApplicationService_Delete"}},
	{"create application", []string{"ApplicationService_Create"}},
	{"update application spec", []string{"ApplicationService_UpdateSpec"}},
	{"patch application", []string{"ApplicationService_Patch"}},
	{"application logs", []string{"ApplicationService_PodLogs", "ApplicationService_PodLogs2"}},
	{"pod logs", []string{"ApplicationService_PodLogs", "ApplicationService_PodLogs2"}},
	{"get logs", []string{"ApplicationService_PodLogs", "ApplicationService_PodLogs2"}},
	{"application resource tree", []string{"ApplicationService_ResourceTree"}},
	{"application events", []string{"ApplicationService_ListResourceEvents"}},
	{"kubernetes events", []string{"ApplicationService_ListResourceEvents"}},
	{"application manifests", []string{"ApplicationService_GetManifests"}},
	{"rendered manifests", []string{"ApplicationService_GetManifests"}},
	{"application diff", []string{"ApplicationService_ServerSideDiff"}},
	{"terminate operation", []string{"ApplicationService_TerminateOperation"}},
	{"cancel running sync", []string{"ApplicationService_TerminateOperation"}},
	{"managed resources", []string{"ApplicationService_ManagedResources"}},
	{"run resource action", []string{"ApplicationService_RunResourceAction", "ApplicationService_RunResourceActionV2"}},
	{"list resource actions", []string{"ApplicationService_ListResourceActions"}},
	{"get resource", []string{"ApplicationService_GetResource"}},
	{"live resource manifest", []string{"ApplicationService_GetResource"}},
	{"delete resource", []string{"ApplicationService_DeleteResource"}},
	{"application sync windows", []string{"ApplicationService_GetApplicationSyncWindows"}},
	{"revision metadata", []string{"ApplicationService_RevisionMetadata"}},
	{"commit author message", []string{"ApplicationService_RevisionMetadata"}},
	{"chart details", []string{"ApplicationService_RevisionChartDetails"}},
	{"watch applications", []string{"ApplicationService_Watch"}},
	{"application links", []string{"ApplicationService_ListLinks"}},
	{"list applicationsets", []string{"ApplicationSetService_List"}},
	{"get applicationset", []string{"ApplicationSetService_Get"}},
	{"delete application set", []string{"ApplicationSetService_Delete"}},
	{"applicationset generate", []string{"ApplicationSetService_Generate"}},
	{"list clusters", []string{"ClusterService_List"}},
	{"get cluster", []string{"ClusterService_Get"}},
	{"invalidate cluster cache", []string{"ClusterService_InvalidateCache"}},
	{"rotate cluster token", []string{"ClusterService_RotateAuth"}},
	{"list projects", []string{"ProjectService_List"}},
	{"get project", []string{"ProjectService_Get"}},
	{"create project", []string{"ProjectService_Create"}},
	{"create project token", []string{"ProjectService_CreateToken"}},
	{"project events", []string{"ProjectService_ListEvents"}},
	{"project sync windows", []string{"ProjectService_GetSyncWindowsState"}},
	{"list repositories", []string{"RepositoryService_ListRepositories"}},
	{"add repository", []string{"RepositoryService_CreateRepository"}},
	{"connect git repository", []string{"RepositoryService_CreateRepository"}},
	{"repository credentials", []string{"RepoCredsService_ListRepositoryCredentials"}},
	{"helm charts", []string{"RepositoryService_GetHelmCharts"}},
	{"git refs", []string{"RepositoryService_ListRefs"}},
	{"oci tags", []string{"RepositoryService_ListOCITags"}},
	{"validate repository access", []string{"RepositoryService_ValidateAccess"}},
	{"apps in repository", []string{"RepositoryService_ListApps"}},
	{"list certificates", []string{"CertificateService_ListCertificates"}},
	{"gpg keys", []string{"GPGKeyService_List"}},
	{"notification triggers", []string{"NotificationService_ListTriggers"}},
	{"notification templates", []string{"NotificationService_ListTemplates"}},
	{"notification services", []string{"NotificationService_ListServices"}},
	{"current user info", []string{"SessionService_GetUserInfo"}},
	{"check permissions", []string{"AccountService_CanI"}},
	{"can i", []string{"AccountService_CanI"}},
	{"list accounts", []string{"AccountService_ListAccounts"}},
	{"create account token", []string{"AccountService_CreateToken"}},
	{"change password", []string{"AccountService_UpdatePassword"}},
	{"argocd version", []string{"VersionService_Version"}},
	{"server version", []string{"VersionService_Version"}},
	{"argocd settings", []string{"SettingsService_Get"}},
	{"plugins", []string{"SettingsService_GetPlugins"}},
	{"ApplicationService_Sync", []string{"ApplicationService_Sync"}},
	{"/api/v1/applications/{name}/sync", []string{"ApplicationService_Sync"}},
}

func loadArgoCDSpec(t *testing.T) []Endpoint {
	t.Helper()
	raw, err := os.ReadFile("testdata/argocd-swagger.json")
	if err != nil {
		t.Fatal(err)
	}
	eps, err := ParseSpec(raw)
	if err != nil {
		t.Fatal(err)
	}
	return eps
}

func rankOf(results []Endpoint, want []string) int {
	for i, ep := range results {
		if slices.Contains(want, ep.OperationID) {
			return i + 1
		}
	}
	return 0
}

func TestSearchRetrieval_ArgoCDSpec(t *testing.T) {
	eps := loadArgoCDSpec(t)
	for _, tc := range retrievalCases {
		if r := rankOf(Search(eps, tc.query, 5), tc.want); r == 0 {
			t.Errorf("%q: none of %v in the top 5, got %v", tc.query, tc.want, operationIDs(Search(eps, tc.query, 5)))
		}
	}
}

func TestSearch_RequiredTerm(t *testing.T) {
	eps := loadArgoCDSpec(t)
	results := Search(eps, "+cluster list", 20)
	if len(results) == 0 {
		t.Fatal("no results")
	}
	for _, ep := range results {
		if !strings.Contains(strings.ToLower(ep.OperationID+ep.Path+ep.Summary+ep.Description), "cluster") {
			t.Errorf("%s returned without the required term", ep.OperationID)
		}
	}
	if results[0].OperationID != "ClusterService_List" {
		t.Errorf("top result = %s, want ClusterService_List", results[0].OperationID)
	}
}

func TestSearch_ExactOperation(t *testing.T) {
	eps := loadArgoCDSpec(t)
	for _, q := range []string{"ApplicationService_Sync", "applicationservice_sync", "/api/v1/applications/{name}/sync"} {
		got := operationIDs(Search(eps, q, 20))
		if !slices.Equal(got, []string{"ApplicationService_Sync"}) {
			t.Errorf("%q = %v, want only ApplicationService_Sync", q, got)
		}
	}
}

func operationIDs(eps []Endpoint) []string {
	out := make([]string, len(eps))
	for i, ep := range eps {
		out[i] = ep.OperationID
	}
	return out
}
