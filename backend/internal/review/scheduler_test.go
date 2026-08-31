package review

import (
	"context"
	"testing"
	"time"
)

func TestSimpleV1Good(t *testing.T) {
	s := NewSimpleV1()
	in := ReviewInput{WrongCount: 1, CorrectCount: 0, ReviewCount: 0, MasteryScore: 0, Result: "good"}
	r, err := s.Calculate(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if r.IntervalDays != 1 {
		t.Fatalf("first good should be 1 day, got %v", r.IntervalDays)
	}
	if r.MasteryScore != 10 {
		t.Fatalf("mastery should be 10, got %v", r.MasteryScore)
	}
	if !r.NextReviewAt.After(time.Now()) {
		t.Fatal("next review should be in future")
	}
}

func TestSimpleV1Again(t *testing.T) {
	s := NewSimpleV1()
	in := ReviewInput{WrongCount: 2, CorrectCount: 1, ReviewCount: 3, MasteryScore: 60, Result: "again"}
	r, err := s.Calculate(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if r.IntervalDays > 0.1 {
		t.Fatalf("again should shorten interval, got %v", r.IntervalDays)
	}
	if r.MasteryScore >= 60 {
		t.Fatalf("mastery should drop, got %v", r.MasteryScore)
	}
	if r.Status != "learning" {
		t.Fatalf("status should be learning, got %s", r.Status)
	}
}

func TestSimpleV1Mastered(t *testing.T) {
	s := NewSimpleV1()
	in := ReviewInput{WrongCount: 1, CorrectCount: 5, ReviewCount: 5, MasteryScore: 90, Result: "easy"}
	r, err := s.Calculate(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "mastered" {
		t.Fatalf("should be mastered, got %s", r.Status)
	}
	if r.MasteryScore <= 90 {
		t.Fatalf("easy should increase mastery, got %v", r.MasteryScore)
	}
}
