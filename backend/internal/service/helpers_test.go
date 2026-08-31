package service

import (
	"github.com/quiztrace/quiztrace/internal/domain"
	"github.com/quiztrace/quiztrace/internal/review"
)

func reviewScheduler() review.ReviewScheduler {
	return review.NewSimpleV1()
}

func reviewInputFor(w *domain.WrongQuestion) review.ReviewInput {
	return review.ReviewInput{
		WrongCount:   w.WrongCount,
		CorrectCount: w.CorrectCount,
		ReviewCount:  w.ReviewCount,
		MasteryScore: w.MasteryScore,
		IntervalDays: w.IntervalDays,
		Difficulty:   3,
		Result:       "good",
	}
}
