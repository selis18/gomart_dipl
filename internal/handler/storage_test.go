package handler

import "context"

func (f loginStorageFunc) AddOrder(context.Context, string, string) error {
	panic("unexpected order insertion")
}
func (f registerStorageFunc) AddOrder(context.Context, string, string) error {
	panic("unexpected order insertion")
}
