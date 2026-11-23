package opsocket

import (
	"reflect"
	"testing"
)

func TestOnlineSlotsSortsDedupesAndIgnoresNonContestants(t *testing.T) {
	clients := []*Client{
		{Principal: Principal{Claims: map[string]string{"slot": "3"}}},
		{Principal: Principal{Claims: map[string]string{"slot": "1"}}},
		{Principal: Principal{Claims: map[string]string{"slot": "3"}}},
		{Principal: Principal{Claims: map[string]string{"slot": "x"}}},
		{Principal: Principal{Claims: map[string]string{"slot": "0"}}},
		{Principal: Principal{Subject: "admin"}},
		nil,
	}
	if got, want := onlineSlots(clients), []int{1, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("onlineSlots = %v, want %v", got, want)
	}
}

func TestOnlineSlotsEmptyIsNotNil(t *testing.T) {
	if got := onlineSlots(nil); got == nil || len(got) != 0 {
		t.Fatalf("onlineSlots(nil) = %#v, want empty non-nil slice (JSON [])", got)
	}
}
