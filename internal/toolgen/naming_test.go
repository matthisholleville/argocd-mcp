package toolgen

import (
	"testing"
)

func TestToToolName_Standard(t *testing.T) {
	tests := []struct {
		operationID string
		want        string
	}{
		{"ApplicationService_List", "argocd_application_list"},
		{"ApplicationService_Create", "argocd_application_create"},
		{"ApplicationService_Get", "argocd_application_get"},
		{"ApplicationService_Delete", "argocd_application_delete"},
		{"ApplicationService_Sync", "argocd_application_sync"},
		{"ClusterService_List", "argocd_cluster_list"},
		{"ClusterService_Get", "argocd_cluster_get"},
		{"ProjectService_Create", "argocd_project_create"},
	}

	for _, tt := range tests {
		t.Run(tt.operationID, func(t *testing.T) {
			got := ToToolName(tt.operationID)
			if got != tt.want {
				t.Errorf("ToToolName(%q) = %q, want %q", tt.operationID, got, tt.want)
			}
		})
	}
}

func TestToToolName_MultiWord(t *testing.T) {
	tests := []struct {
		operationID string
		want        string
	}{
		{"ApplicationService_GetResource", "argocd_application_get_resource"},
		{"ApplicationService_PatchResource", "argocd_application_patch_resource"},
		{"ApplicationService_DeleteResource", "argocd_application_delete_resource"},
		{"ApplicationService_ListResourceActions", "argocd_application_list_resource_actions"},
		{"ApplicationService_RunResourceAction", "argocd_application_run_resource_action"},
		{"ApplicationService_PodLogs", "argocd_application_pod_logs"},
		{"ApplicationService_ListResourceEvents", "argocd_application_list_resource_events"},
		{"ApplicationService_ManagedResources", "argocd_application_managed_resources"},
		{"ApplicationService_ResourceTree", "argocd_application_resource_tree"},
		{"ApplicationService_TerminateOperation", "argocd_application_terminate_operation"},
		{"ApplicationService_RevisionMetadata", "argocd_application_revision_metadata"},
		{"ApplicationService_RevisionChartDetails", "argocd_application_revision_chart_details"},
		{"ApplicationService_ServerSideDiff", "argocd_application_server_side_diff"},
		{"ApplicationService_GetApplicationSyncWindows", "argocd_application_get_application_sync_windows"},
		{"ClusterService_InvalidateCache", "argocd_cluster_invalidate_cache"},
		{"ClusterService_RotateAuth", "argocd_cluster_rotate_auth"},
		{"ProjectService_GetDetailedProject", "argocd_project_get_detailed_project"},
		{"ProjectService_GetSyncWindowsState", "argocd_project_get_sync_windows_state"},
	}

	for _, tt := range tests {
		t.Run(tt.operationID, func(t *testing.T) {
			got := ToToolName(tt.operationID)
			if got != tt.want {
				t.Errorf("ToToolName(%q) = %q, want %q", tt.operationID, got, tt.want)
			}
		})
	}
}

func TestToToolName_ApplicationSet(t *testing.T) {
	tests := []struct {
		operationID string
		want        string
	}{
		{"ApplicationSetService_List", "argocd_application_set_list"},
		{"ApplicationSetService_Create", "argocd_application_set_create"},
		{"ApplicationSetService_Get", "argocd_application_set_get"},
		{"ApplicationSetService_Delete", "argocd_application_set_delete"},
		{"ApplicationSetService_ResourceTree", "argocd_application_set_resource_tree"},
	}

	for _, tt := range tests {
		t.Run(tt.operationID, func(t *testing.T) {
			got := ToToolName(tt.operationID)
			if got != tt.want {
				t.Errorf("ToToolName(%q) = %q, want %q", tt.operationID, got, tt.want)
			}
		})
	}
}

func TestToToolName_UppercaseAcronyms(t *testing.T) {
	tests := []struct {
		operationID string
		want        string
	}{
		{"ApplicationService_GetOCIMetadata", "argocd_application_get_oci_metadata"},
		{"GPGKeyService_List", "argocd_gpg_key_list"},
		{"GPGKeyService_Create", "argocd_gpg_key_create"},
	}

	for _, tt := range tests {
		t.Run(tt.operationID, func(t *testing.T) {
			got := ToToolName(tt.operationID)
			if got != tt.want {
				t.Errorf("ToToolName(%q) = %q, want %q", tt.operationID, got, tt.want)
			}
		})
	}
}

func TestToToolName_NoServiceSuffix(t *testing.T) {
	got := ToToolName("CustomOp")
	want := "argocd_custom_op"
	if got != want {
		t.Errorf("ToToolName(%q) = %q, want %q", "CustomOp", got, want)
	}
}

func TestToToolName_Empty(t *testing.T) {
	got := ToToolName("")
	if got != "" {
		t.Errorf("ToToolName(%q) = %q, want empty", "", got)
	}
}

func TestToToolName_NoUnderscore(t *testing.T) {
	got := ToToolName("ListApplications")
	want := "argocd_list_applications"
	if got != want {
		t.Errorf("ToToolName(%q) = %q, want %q", "ListApplications", got, want)
	}
}

func TestToToolName_PodLogs2(t *testing.T) {
	got := ToToolName("ApplicationService_PodLogs2")
	want := "argocd_application_pod_logs2"
	if got != want {
		t.Errorf("ToToolName(%q) = %q, want %q", "ApplicationService_PodLogs2", got, want)
	}
}

func TestToToolName_RepoCredsService(t *testing.T) {
	tests := []struct {
		operationID string
		want        string
	}{
		{"RepoCredsService_ListRepositoryCredentials", "argocd_repo_creds_list_repository_credentials"},
		{"RepoCredsService_CreateRepositoryCredentials", "argocd_repo_creds_create_repository_credentials"},
	}

	for _, tt := range tests {
		t.Run(tt.operationID, func(t *testing.T) {
			got := ToToolName(tt.operationID)
			if got != tt.want {
				t.Errorf("ToToolName(%q) = %q, want %q", tt.operationID, got, tt.want)
			}
		})
	}
}

func TestToToolName_NotificationService(t *testing.T) {
	tests := []struct {
		operationID string
		want        string
	}{
		{"NotificationService_ListServices", "argocd_notification_list_services"},
		{"NotificationService_ListTemplates", "argocd_notification_list_templates"},
		{"NotificationService_ListTriggers", "argocd_notification_list_triggers"},
	}

	for _, tt := range tests {
		t.Run(tt.operationID, func(t *testing.T) {
			got := ToToolName(tt.operationID)
			if got != tt.want {
				t.Errorf("ToToolName(%q) = %q, want %q", tt.operationID, got, tt.want)
			}
		})
	}
}

func TestToToolName_CertificateService(t *testing.T) {
	tests := []struct {
		operationID string
		want        string
	}{
		{"CertificateService_ListCertificates", "argocd_certificate_list_certificates"},
		{"CertificateService_CreateCertificate", "argocd_certificate_create_certificate"},
		{"CertificateService_DeleteCertificate", "argocd_certificate_delete_certificate"},
	}

	for _, tt := range tests {
		t.Run(tt.operationID, func(t *testing.T) {
			got := ToToolName(tt.operationID)
			if got != tt.want {
				t.Errorf("ToToolName(%q) = %q, want %q", tt.operationID, got, tt.want)
			}
		})
	}
}

func TestToToolName_AccountService(t *testing.T) {
	tests := []struct {
		operationID string
		want        string
	}{
		{"AccountService_ListAccounts", "argocd_account_list_accounts"},
		{"AccountService_GetAccount", "argocd_account_get_account"},
		{"AccountService_CanI", "argocd_account_can_i"},
		{"AccountService_UpdatePassword", "argocd_account_update_password"},
		{"AccountService_CreateToken", "argocd_account_create_token"},
		{"AccountService_DeleteToken", "argocd_account_delete_token"},
	}

	for _, tt := range tests {
		t.Run(tt.operationID, func(t *testing.T) {
			got := ToToolName(tt.operationID)
			if got != tt.want {
				t.Errorf("ToToolName(%q) = %q, want %q", tt.operationID, got, tt.want)
			}
		})
	}
}

func TestDeduplicateNames(t *testing.T) {
	input := []string{
		"argocd_application_list",
		"argocd_cluster_list",
		"argocd_application_list", // dup
		"argocd_project_get",
		"argocd_application_list", // dup again
	}

	got := DeduplicateNames(input)
	want := []string{
		"argocd_application_list",
		"argocd_cluster_list",
		"argocd_application_list_2",
		"argocd_project_get",
		"argocd_application_list_3",
	}

	if len(got) != len(want) {
		t.Fatalf("length mismatch: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDeduplicateNames_NoDuplicates(t *testing.T) {
	input := []string{"a", "b", "c"}
	got := DeduplicateNames(input)
	for i := range input {
		if got[i] != input[i] {
			t.Errorf("index %d: got %q, want %q", i, got[i], input[i])
		}
	}
}

func TestDeduplicateNames_Empty(t *testing.T) {
	got := DeduplicateNames(nil)
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestDeduplicateNames_EmptyStringsPreserved(t *testing.T) {
	input := []string{"", "a", "", "b", ""}
	got := DeduplicateNames(input)
	want := []string{"", "a", "", "b", ""}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCamelToSnake(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"InvalidateCache", "invalidate_cache"},
		{"ApplicationSet", "application_set"},
		{"PodLogs2", "pod_logs2"},
		{"Get", "get"},
		{"OCIMetadata", "oci_metadata"},
		{"GPGKey", "gpg_key"},
		{"CanI", "can_i"},
		{"List", "list"},
		{"ServerSideDiff", "server_side_diff"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := camelToSnake(tt.input)
			if got != tt.want {
				t.Errorf("camelToSnake(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
