package service

import (
	"reflect"
	"testing"
)

func TestMarkerNumbersDetectsTailAndResetBlock(t *testing.T) {
	text := "83. 题干\n84, 尾题\n二、多项选择题\n1《中国教育现代化》\n2. 第二题\nA. 选项"
	if got, want := markerNumbers(text), []int{83, 84, 1, 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestMultisetDifferenceDetectsWholeOmittedBlock(t *testing.T) {
	got := multisetDifference([]int{77, 78, 84, 1, 2}, []int{77, 78})
	want := []int{84, 1, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}
