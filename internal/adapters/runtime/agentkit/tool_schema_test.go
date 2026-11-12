package agentkit

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"fkteams/internal/adapters/tools/builtin/excel"
	"fkteams/internal/adapters/tools/builtin/scheduler"
	runtimeport "fkteams/internal/ports/runtime"
)

func TestBuiltinSchemasMatchActualJSONContracts(t *testing.T) {
	et, err := excel.NewExcelTools(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	excelTools, err := et.GetTools()
	if err != nil {
		t.Fatal(err)
	}
	scheduleTools, err := scheduler.NewTools(nil).GetTools()
	if err != nil {
		t.Fatal(err)
	}
	tools, err := AdaptToolsForRunner(context.Background(), append(excelTools, scheduleTools...))
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools {
		info, err := tool.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		s, err := info.ParamsOneOf.ToJSONSchema()
		if err != nil {
			t.Fatal(err)
		}
		switch info.Name {
		case "excel_batch_fill_cols":
			field, _ := s.Properties.Get("columns")
			if field.Type != "object" || field.AdditionalProperties == nil {
				t.Fatalf("map schema: %#v", field)
			}
		case "excel_set_cell_value":
			field, _ := s.Properties.Get("value")
			if field.Type != "" {
				t.Fatalf("any restricted to %q", field.Type)
			}
		case "schedule_update":
			if _, ok := s.Properties.Get("ScheduleAddRequest"); ok {
				t.Fatal("anonymous field was not flattened")
			}
			if _, ok := s.Properties.Get("task"); !ok {
				t.Fatal("flattened task missing")
			}
		}
	}
	request := scheduler.ScheduleUpdateRequest{TaskID: "id", ScheduleAddRequest: scheduler.ScheduleAddRequest{Task: "task", ExecuteAt: "2030-01-01T00:00:00Z"}}
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var decoded scheduler.ScheduleUpdateRequest
	if err := json.Unmarshal(data, &decoded); err != nil || decoded.Task != "task" {
		t.Fatalf("decode mismatch: %#v %v", decoded, err)
	}
	s := schemaForType(reflect.TypeOf(struct {
		Count int `json:"count"`
	}{}))
	count, _ := s.Properties.Get("count")
	if count.Type != "integer" {
		t.Fatalf("integer schema: %#v", count)
	}
	// 经过工具装配后仍保留有类型的入参提供者。
	if _, ok := excelTools[0].(runtimeport.ToolInputTypeProvider); !ok {
		t.Fatal("input contract lost")
	}
}
