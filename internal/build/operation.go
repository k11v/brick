package build

import (
	"iter"
	"time"

	"github.com/google/uuid"
)

type Operation struct {
	ID                uuid.UUID
	UserID            uuid.UUID
	InputDirPrefix    string
	OutputPDFFileKey  string
	ProcessLogFileKey string
	ProcessExitCode   int
	CreatedAt         time.Time
}

type OperationService struct{}

type CreateBuildParams struct {
	UserID uuid.UUID
	Files  iter.Seq2[File, error]
}

func (*OperationService) CreateBuild(params *CreateBuildParams) (*Operation, error) {
	return &Operation{}, nil
}
