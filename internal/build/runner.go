package build

import (
	"fmt"

	"github.com/google/uuid"
)

type OperationRunner struct{}

type OperationRunnerRunParams struct {
	ID     uuid.UUID
	UserID uuid.UUID
}

type OperationRunnerRunResult struct{}

func (r *OperationRunner) Run(params *OperationRunnerRunParams) (*OperationRunnerRunResult, error) {
	runResult, err := Run(&RunParams{
		InputDir:  "",
		OutputDir: "",
	})
	if err != nil {
		return nil, fmt.Errorf("build.OperationRunner: %w", err)
	}
	_ = runResult

	return &OperationRunnerRunResult{}, nil
}
