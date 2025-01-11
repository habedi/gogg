package helpers

import "testing"

func TestAddPositiveNumbers(t *testing.T) {
	result := Add(2, 3)
	if result != 5 {
		t.Errorf("Expected 5, but got %d", result)
	}
}

func TestAddNegativeNumbers(t *testing.T) {
	result := Add(-2, -3)
	if result != -5 {
		t.Errorf("Expected -5, but got %d", result)
	}
}

func TestAddPositiveAndNegativeNumber(t *testing.T) {
	result := Add(2, -3)
	if result != -1 {
		t.Errorf("Expected -1, but got %d", result)
	}
}

func TestAddZero(t *testing.T) {
	result := Add(0, 0)
	if result != 0 {
		t.Errorf("Expected 0, but got %d", result)
	}
}

func TestAddLargeNumbers(t *testing.T) {
	result := Add(1000000, 2000000)
	if result != 3000000 {
		t.Errorf("Expected 3000000, but got %d", result)
	}
}
