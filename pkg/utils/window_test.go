package utils

import (
	"math"
	"testing"
)

func TestWindowInsertAndMean(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		size     int
		values   []float64
		wantMean float64
	}{
		{
			name:     "single value",
			size:     5,
			values:   []float64{10},
			wantMean: 10,
		},
		{
			name:     "partial fill",
			size:     5,
			values:   []float64{1, 2, 3},
			wantMean: 2,
		},
		{
			name:     "full window",
			size:     3,
			values:   []float64{2, 4, 6},
			wantMean: 4,
		},
		{
			name:     "overwrite oldest",
			size:     3,
			values:   []float64{10, 20, 30, 40},
			wantMean: 30, // [40, 20, 30] circular → mean of 20,30,40
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := NewWindow(tt.size)
			for _, v := range tt.values {
				w.Insert(v)
			}
			got := w.Mean()
			if math.Abs(got-tt.wantMean) > 0.001 {
				t.Errorf("Mean() = %f, want %f", got, tt.wantMean)
			}
		})
	}
}

func TestWindowIsFull(t *testing.T) {
	t.Parallel()
	w := NewWindow(3)
	if w.IsFull() {
		t.Error("IsFull() should be false on empty window")
	}
	w.Insert(1)
	w.Insert(2)
	if w.IsFull() {
		t.Error("IsFull() should be false before filling")
	}
	w.Insert(3)
	if !w.IsFull() {
		t.Error("IsFull() should be true after filling")
	}
}

func TestWindowIsEmpty(t *testing.T) {
	t.Parallel()
	w := NewWindow(5)
	if !w.IsEmpty() {
		t.Error("IsEmpty() should be true on new window")
	}
	w.Insert(1)
	if w.IsEmpty() {
		t.Error("IsEmpty() should be false after insert")
	}
}

func TestWindowAbsMax(t *testing.T) {
	t.Parallel()
	w := NewWindow(5)
	w.Insert(-100)
	w.Insert(50)
	w.Insert(-30)
	if got := w.AbsMax(); got != 100 {
		t.Errorf("AbsMax() = %f, want 100", got)
	}
}

func TestWindowLastInserted(t *testing.T) {
	t.Parallel()
	w := NewWindow(3)
	w.Insert(5)
	w.Insert(10)
	w.Insert(15)
	if got := w.LastInserted(); got != 15 {
		t.Errorf("LastInserted() = %f, want 15", got)
	}
	w.Insert(20)
	if got := w.LastInserted(); got != 20 {
		t.Errorf("LastInserted() = %f, want 20 after wrap", got)
	}
}

func TestWindowCircularOverwrite(t *testing.T) {
	t.Parallel()
	w := NewWindow(3)
	w.Insert(1)
	w.Insert(2)
	w.Insert(3)
	if !w.IsFull() {
		t.Fatal("expected full")
	}
	w.Insert(100)
	if !w.IsFull() {
		t.Error("should still be full after overwrite")
	}
	// After overwrite: [100, 2, 3] → mean should be (100+2+3)/3 = 35
	got := w.Mean()
	expected := 35.0
	if math.Abs(got-expected) > 0.001 {
		t.Errorf("Mean after overwrite = %f, want %f", got, expected)
	}
}
