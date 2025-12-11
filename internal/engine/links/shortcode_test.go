package links

import (
	"errors"
	"testing"
)

type MockChecker struct {
	codes map[string]bool
}

func (m *MockChecker) ExistsByShortCode(code string) (bool, error) {
	if code == "error" {
		return false, errors.New("db error")
	}
	return m.codes[code], nil
}

func TestGenerateShortCode(t *testing.T) {
	checker := &MockChecker{
		codes: map[string]bool{
			"taken": true,
		},
	}

	// Test Case 1: Custom Code Success
	code, err := GenerateShortCode("custom", checker)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if code != "custom" {
		t.Errorf("Expected custom, got %s", code)
	}

	// Test Case 2: Custom Code Taken
	_, err = GenerateShortCode("taken", checker)
	if err == nil {
		t.Error("Expected error for taken code, got nil")
	}

	// Test Case 3: Random Code Generation
	code, err = GenerateShortCode("", checker)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(code) != shortCodeLength {
		t.Errorf("Expected length %d, got %d", shortCodeLength, len(code))
	}
}
