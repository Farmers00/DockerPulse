package database

import (
	"testing"
	"time"
)

func TestDeduplicateStacks(t *testing.T) {
	tmpDir := t.TempDir()

	db, err := InitDB(tmpDir)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer db.Close()

	host := &Host{
		Name:    "Test Host",
		Driver:  DriverSocket,
		Address: "local",
		BaseDir: "/root/docker",
		Status:  "online",
	}
	if err := db.CreateHost(host); err != nil {
		t.Fatalf("failed to create host: %v", err)
	}

	// Insert duplicate stacks: one legacy container path, one host path
	s1 := &Stack{
		HostID: host.ID,
		Name:   "emby",
		Path:   "/root/docker/emby",
		Status: "discovered",
	}
	if err := db.UpsertStack(s1); err != nil {
		t.Fatalf("failed to insert s1: %v", err)
	}

	s2 := &Stack{
		HostID: host.ID,
		Name:   "emby",
		Path:   "/home/farmers00/docker/emby",
		Status: "discovered",
	}
	if err := db.UpsertStack(s2); err != nil {
		t.Fatalf("failed to upsert s2: %v", err)
	}

	// Verify only 1 stack remains and its path is the host path
	stacks, err := db.ListStacks(host.ID)
	if err != nil {
		t.Fatalf("failed to list stacks: %v", err)
	}
	if len(stacks) != 1 {
		t.Fatalf("expected 1 stack after upsert, got %d", len(stacks))
	}
	if stacks[0].Path != "/home/farmers00/docker/emby" {
		t.Fatalf("expected path /home/farmers00/docker/emby, got %s", stacks[0].Path)
	}
}

func TestInitDBDeduplication(t *testing.T) {
	tmpDir := t.TempDir()

	db, err := InitDB(tmpDir)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}

	host := &Host{
		Name:    "Test Host",
		Driver:  DriverSocket,
		Address: "local",
		BaseDir: "/root/docker",
		Status:  "online",
	}
	_ = db.CreateHost(host)

	now := time.Now().UTC()
	// Manually insert two duplicates bypassing UpsertStack
	_, _ = db.conn.Exec(`
		INSERT INTO stacks (id, host_id, name, path, status, auto_update, created_at, updated_at)
		VALUES ('id1', ?, 'homepage', '/root/docker/homepage', 'discovered', 0, ?, ?)
	`, host.ID, now, now)

	_, _ = db.conn.Exec(`
		INSERT INTO stacks (id, host_id, name, path, status, auto_update, created_at, updated_at)
		VALUES ('id2', ?, 'homepage', '/home/farmers00/docker/homepage', 'discovered', 0, ?, ?)
	`, host.ID, now, now)

	db.Close()

	// Re-open DB to trigger InitDB deduplication migration
	db2, err := InitDB(tmpDir)
	if err != nil {
		t.Fatalf("failed to re-open db: %v", err)
	}
	defer db2.Close()

	stacks, err := db2.ListStacks(host.ID)
	if err != nil {
		t.Fatalf("failed to list stacks: %v", err)
	}
	if len(stacks) != 1 {
		t.Fatalf("expected 1 stack after InitDB deduplication, got %d", len(stacks))
	}
	if stacks[0].Path != "/home/farmers00/docker/homepage" {
		t.Fatalf("expected path /home/farmers00/docker/homepage, got %s", stacks[0].Path)
	}
}
