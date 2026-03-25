package artifact

import (
	"context"
	"strings"
	"testing"
)

func TestStore_CreateAndList(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()
	execID := createTestExecution(t, database)

	store := &Store{DB: database}

	a := &Artifact{
		ExecutionID: execID,
		StepName:    "fetch",
		Name:        "output.json",
		URL:         "s3://bucket/output.json",
		Size:        1024,
	}
	if err := store.Create(ctx, a); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	artifacts, err := store.ListByExecution(ctx, execID)
	if err != nil {
		t.Fatalf("ListByExecution() error = %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("ListByExecution() returned %d artifacts, want 1", len(artifacts))
	}

	got := artifacts[0]
	if got.ExecutionID != execID {
		t.Errorf("ExecutionID = %q, want %q", got.ExecutionID, execID)
	}
	if got.StepName != "fetch" {
		t.Errorf("StepName = %q, want %q", got.StepName, "fetch")
	}
	if got.Name != "output.json" {
		t.Errorf("Name = %q, want %q", got.Name, "output.json")
	}
	if got.URL != "s3://bucket/output.json" {
		t.Errorf("URL = %q, want %q", got.URL, "s3://bucket/output.json")
	}
	if got.Size != 1024 {
		t.Errorf("Size = %d, want 1024", got.Size)
	}
	if got.ID == "" {
		t.Error("ID is empty, want a UUID")
	}
}

func TestStore_GetByName(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()
	execID := createTestExecution(t, database)

	store := &Store{DB: database}

	a := &Artifact{
		ExecutionID: execID,
		StepName:    "transform",
		Name:        "result.csv",
		URL:         "s3://bucket/result.csv",
		Size:        2048,
	}
	if err := store.Create(ctx, a); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := store.GetByName(ctx, execID, "result.csv")
	if err != nil {
		t.Fatalf("GetByName() error = %v", err)
	}

	if got.ExecutionID != execID {
		t.Errorf("ExecutionID = %q, want %q", got.ExecutionID, execID)
	}
	if got.StepName != "transform" {
		t.Errorf("StepName = %q, want %q", got.StepName, "transform")
	}
	if got.Name != "result.csv" {
		t.Errorf("Name = %q, want %q", got.Name, "result.csv")
	}
	if got.URL != "s3://bucket/result.csv" {
		t.Errorf("URL = %q, want %q", got.URL, "s3://bucket/result.csv")
	}
	if got.Size != 2048 {
		t.Errorf("Size = %d, want 2048", got.Size)
	}
	if got.ID == "" {
		t.Error("ID is empty, want a UUID")
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero, want a non-zero time")
	}
}

func TestStore_DuplicateNameFails(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()
	execID := createTestExecution(t, database)

	store := &Store{DB: database}

	a := &Artifact{
		ExecutionID: execID,
		StepName:    "step1",
		Name:        "report.pdf",
		URL:         "s3://bucket/report.pdf",
		Size:        512,
	}
	if err := store.Create(ctx, a); err != nil {
		t.Fatalf("Create() first artifact error = %v", err)
	}

	duplicate := &Artifact{
		ExecutionID: execID,
		StepName:    "step2",
		Name:        "report.pdf",
		URL:         "s3://bucket/report-v2.pdf",
		Size:        768,
	}
	err := store.Create(ctx, duplicate)
	if err == nil {
		t.Fatal("Create() second artifact with duplicate name expected error, got nil")
	}
	if !strings.Contains(err.Error(), "artifact create") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "artifact create")
	}
	if !strings.Contains(err.Error(), "duplicate key") && !strings.Contains(err.Error(), "unique") {
		t.Errorf("error = %q, want it to indicate a uniqueness violation", err.Error())
	}
}
