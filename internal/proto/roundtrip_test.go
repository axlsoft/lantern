package proto_test

import (
	"testing"

	lanternv1 "github.com/axlsoft/lantern/gen/proto/lantern/v1"
	"google.golang.org/protobuf/proto"
)

func TestCoverageBatchRoundTrip(t *testing.T) {
	original := &lanternv1.CoverageBatch{
		Resource: &lanternv1.Resource{
			ProjectId:     "proj-123",
			RunId:         "run-456",
			CommitSha:     "abc123def456",
			Branch:        "main",
			SdkName:       "lantern-dotnet",
			SdkVersion:    "1.0.0",
			SchemaVersion: "1",
		},
		BatchId: "batch-789",
		Events: []*lanternv1.Coverage{
			{
				FilePath:        "src/MyService.cs",
				LineStart:       10,
				LineEnd:         20,
				HitCount:        3,
				TestId:          "test-001",
				AttributionMode: lanternv1.AttributionMode_ATTRIBUTION_MODE_SERIALIZED,
			},
		},
	}

	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	decoded := &lanternv1.CoverageBatch{}
	if err := proto.Unmarshal(data, decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.BatchId != original.BatchId {
		t.Errorf("batch_id: got %q, want %q", decoded.BatchId, original.BatchId)
	}
	if decoded.Resource.SchemaVersion != "1" {
		t.Errorf("schema_version: got %q, want %q", decoded.Resource.SchemaVersion, "1")
	}
	if len(decoded.Events) != 1 {
		t.Fatalf("events: got %d, want 1", len(decoded.Events))
	}
	if decoded.Events[0].FilePath != "src/MyService.cs" {
		t.Errorf("file_path: got %q", decoded.Events[0].FilePath)
	}
}
