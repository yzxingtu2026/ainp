package aitool

import (
	"context"
	"encoding/json"
	"fmt"
)

type StaticProvider struct {
	ProviderSource Source
	Definitions    []Definition
}

func (p *StaticProvider) Source() Source { return p.ProviderSource }
func (p *StaticProvider) List(context.Context) ([]Definition, error) {
	return append([]Definition(nil), p.Definitions...), nil
}
func (p *StaticProvider) Get(_ context.Context, id string) (*Definition, error) {
	for _, d := range p.Definitions {
		if d.ID == id {
			copy := d
			return &copy, nil
		}
	}
	return nil, fmt.Errorf("tool not found")
}

type ExecuteFunc func(context.Context, LoadedTool, json.RawMessage) (*Result, error)

func (f ExecuteFunc) Execute(ctx context.Context, t LoadedTool, a json.RawMessage) (*Result, error) {
	return f(ctx, t, a)
}
