package test

import (
	"encoding/json"
	"testing"
	"time"
)

type ComparableS struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

func TestSerialization(t *testing.T) {
	c := NewCache(time.Second)
	v := ComparableS{Id: 1}
	b, _ := json.Marshal(v)
	c.LoadOrStore("yhm", b)
	wrap, ok := c.Get("yhm")
	if !ok {
		t.Error("Get from cache failed")
	}
	bs, ok := wrap.([]byte)
	if !ok {
		t.Error("Type assertion failed")
	}
	var value ComparableS
	if err := json.Unmarshal(bs, &value); err != nil {
		t.Error(err.Error())
	}
	if v != value {
		t.Error("Value not equal")
	}
}

func TestGetAfterExpired(t *testing.T) {
	c := NewCache(10 * time.Second)
	c.LoadOrStore("yhm", 1)
	time.Sleep(11 * time.Second)
	_, ok := c.Get("yhm")
	if ok {
		t.Error("Get from cache success after expired,but it should not")
	}
}

func TestInsertAndGetStruct(t *testing.T) {
	c := NewCache(time.Second)
	v := ComparableS{Id: 1}
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
