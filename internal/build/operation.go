package build

import (
	"iter"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rabbitmq/amqp091-go"
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

type OperationService struct {
	db *pgxpool.Pool
	mq *amqp091.Connection
	s3 *s3.Client
}

type CreateBuildParams struct {
	UserID uuid.UUID
	Files  iter.Seq2[File, error]
}

func (s *OperationService) CreateBuild(params *CreateBuildParams) (*Operation, error) {
	return &Operation{}, nil
}
