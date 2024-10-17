package postgresutil

// Config holds Postgres configuration.
type Config struct {
	DSN string `env:"DSN,required"` // required
}
