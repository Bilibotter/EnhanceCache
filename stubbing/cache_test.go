package stubbing

import (
	"testing"
	"time"
)

type ComparableS struct {
	id int
}

func TestGetAfterExpired(t *testing.T) {
	c := NewCache(time.Second)
	c.LoadOrStore("yhm", 1)
	time.Sleep(2 * time.Second)
	_, ok := c.Get("yhm")
	if ok {
		t.Error("Get from cache success after expired,but it should not")
	}
}

func TestInsertAndGetStruct(t *testing.T) {
	c := NewCache(time.Second)
	v := ComparableS{id: 1}
	c.LoadOrStore("yhm", v)
	wrap, ok := c.Get("yhm")
	if !ok {
		t.Error("Get from cache failed")
	}
	value, ok := wrap.(ComparableS)
	if !ok {
		t.Error("Type assertion failed")
	}
	if v != value {
		t.Error("Value not equal")
	}
}
