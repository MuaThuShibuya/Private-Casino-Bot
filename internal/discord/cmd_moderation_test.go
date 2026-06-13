// internal\discord\cmd_moderation_test.go
package discord

import (
	"reflect"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

func TestValidateDeleteAmount(t *testing.T) {
	cases := []struct {
		name        string
		input       string
		expectError bool
		errType     string
		expectedAmt int
	}{
		{"Valid small amount", "10", false, "", 10},
		{"Valid exactly 100", "100", false, "", 100},
		{"Exceeds limit is rejected", "150", true, "exceeds_limit", 0},
		{"Zero is invalid", "0", true, "invalid", 0},
		{"Negative is invalid", "-5", true, "invalid", 0},
		{"Not a number", "abc", true, "invalid", 0},
		{"Empty string", "", true, "invalid", 0},
		{"With spaces", " 10 ", false, "", 10},
		{"Leading zeros", "0010", false, "", 10},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			amt, err := validateDeleteAmount(tc.input)
			hasError := err != nil
			if hasError != tc.expectError {
				t.Errorf("Expected error: %v, got: %v", tc.expectError, hasError)
			}
			if hasError && err.Error() != tc.errType {
				t.Errorf("Expected err type: %s, got: %s", tc.errType, err.Error())
			}
			if !hasError && amt != tc.expectedAmt {
				t.Errorf("Expected amount: %d, got: %d", tc.expectedAmt, amt)
			}
		})
	}
}

func TestParseDeleteArgs(t *testing.T) {
	cases := []struct {
		name        string
		args        []string
		expectError bool
		expected    DeleteOptions
	}{
		{"No args", []string{}, true, DeleteOptions{}},
		{"Amount only", []string{"20"}, false, DeleteOptions{Amount: 20, IncludeOld: false}},
		{"Amount and --old flag", []string{"20", "--old"}, false, DeleteOptions{Amount: 20, IncludeOld: true}},
		{"Flag first then amount", []string{"old", "50"}, false, DeleteOptions{Amount: 50, IncludeOld: true}},
		{"Invalid argument mixed", []string{"20", "garbage"}, true, DeleteOptions{}},
		{"Exceed limit", []string{"150", "--old"}, true, DeleteOptions{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts, err := ParseDeleteArgs(tc.args)
			hasError := err != nil
			if hasError != tc.expectError {
				t.Errorf("Expected error: %v, got: %v (err: %v)", tc.expectError, hasError, err)
			}
			if !hasError && !reflect.DeepEqual(opts, tc.expected) {
				t.Errorf("Expected opts: %+v, got: %+v", tc.expected, opts)
			}
		})
	}
}

func TestBuildDeletePlan(t *testing.T) {
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	recentTime := now.Add(-5 * 24 * time.Hour)
	oldTime := now.Add(-15 * 24 * time.Hour)
	boundaryTime := now.Add(-14 * 24 * time.Hour)

	messages := []*discordgo.Message{
		{ID: "recent_1", Timestamp: recentTime},
		{ID: "recent_2", Timestamp: recentTime},
		{ID: "old_1", Timestamp: oldTime},
		{ID: "pinned_1", Timestamp: recentTime, Pinned: true},
		{ID: "boundary_1", Timestamp: boundaryTime},
	}

	t.Run("Default Mode (Ignore Old)", func(t *testing.T) {
		plan := BuildDeletePlan(messages, now, false)

		// Kì vọng: Bulk = recent_1, recent_2 (Boundary bị tính là cũ do trừ 1 phút an toàn)
		if len(plan.BulkMessageIDs) != 2 {
			t.Errorf("Expected 2 bulk messages, got %d", len(plan.BulkMessageIDs))
		}
		if len(plan.OldMessageIDs) != 0 {
			t.Errorf("Expected 0 old messages for single delete, got %d", len(plan.OldMessageIDs))
		}
		if plan.SkippedPinned != 1 {
			t.Errorf("Expected 1 skipped pinned, got %d", plan.SkippedPinned)
		}
		if plan.SkippedOld != 2 {
			t.Errorf("Expected 2 skipped old (old_1, boundary_1), got %d", plan.SkippedOld)
		}

		// Tuyệt đối không có pinned hoặc old trong Bulk
		for _, id := range plan.BulkMessageIDs {
			if id == "pinned_1" || id == "old_1" || id == "boundary_1" {
				t.Errorf("BulkMessageIDs contains invalid ID: %s", id)
			}
		}
	})

	t.Run("IncludeOld Mode", func(t *testing.T) {
		plan := BuildDeletePlan(messages, now, true)

		if len(plan.BulkMessageIDs) != 2 {
			t.Errorf("Expected 2 bulk messages, got %d", len(plan.BulkMessageIDs))
		}
		if len(plan.OldMessageIDs) != 2 {
			t.Errorf("Expected 2 old message for single delete, got %d", len(plan.OldMessageIDs))
		}
		if plan.SkippedOld != 0 {
			t.Errorf("Expected 0 skipped old since flag is true, got %d", plan.SkippedOld)
		}
	})
}
