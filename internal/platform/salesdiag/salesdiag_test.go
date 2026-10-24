package salesdiag

import (
	"strings"
	"testing"
)

func TestEnabledDefaultsOffAndRequiresExplicitTrue(t *testing.T) {
	t.Setenv("MUJEEB_SALES_DIAGNOSTICS", "")
	if Enabled() {
		t.Fatal("diagnostics must default off")
	}
	t.Setenv("MUJEEB_SALES_DIAGNOSTICS", "false")
	if Enabled() {
		t.Fatal("diagnostics enabled without true")
	}
	t.Setenv("MUJEEB_SALES_DIAGNOSTICS", " true ")
	if !Enabled() {
		t.Fatal("explicit true did not enable diagnostics")
	}
}

func TestDescribeArgumentsDistinguishesEmptyAndPopulatedObjectWithoutLeakingValues(t *testing.T) {
	empty := `{"sales":{"workflow_id":"workflow-1","expected_version":2,"transaction_type":"subscription","lines":[],"transaction_data_json":"{}","evidence_message_ids":[]}}`
	got := DescribeArguments(empty)
	if !strings.Contains(got, "data_json_empty_object=true") || !strings.Contains(got, "count=0") {
		t.Fatalf("cannot diagnose empty model result: %s", got)
	}
	present := `{"sales":{"workflow_id":"workflow-1","expected_version":2,"transaction_type":"subscription","lines":[],"transaction_data_json":"{\\"field_abc\\":\\"A secret name\\",\\"field_xyz\\":\\"776900067\\"}","evidence_message_ids":[]}}`
	got = DescribeArguments(present)
	if !strings.Contains(got, "data_json_empty_object=false") || !strings.Contains(got, "field_abc") || !strings.Contains(got, "field_xyz") || !strings.Contains(got, "count=2") {
		t.Fatalf("missing required evidence from model result: %s", got)
	}
	if strings.Contains(got, "776900067") || strings.Contains(got, "A secret name") {
		t.Fatalf("model personal data leaked: %s", got)
	}
}

func TestDescribeArgumentsHandlesMissingAndInvalidData(t *testing.T) {
	if got := DescribeArguments("not-json"); got != "args_parse=invalid_json" {
		t.Fatalf("malformed args not identified: %s", got)
	}
	if got := DescribeArguments(`{"sales":{"transaction_data_json":"invalid"}}`); !strings.Contains(got, "data_json_valid_object=false") {
		t.Fatalf("malformed nested data not identified: %s", got)
	}
	if got := DescribeArguments(`{"sales":{}}`); !strings.Contains(got, "data_field=missing") {
		t.Fatalf("missing nested data not identified: %s", got)
	}
}
