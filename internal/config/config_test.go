package config

import (
	"log"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		expCfg *Config
		err    bool
	}{
		{
			name: "complete",
			expCfg: &Config{
				Server: ServerConfig{
					Address: "http://localhost:8200",
				},
				AuthMethods: AuthMethods{
					Enabled: true,
				},
				SecretEngines: SecretEngines{
					Enabled: true,
				},
				Exporter: ExporterConfig{
					CollectionInterval: Duration{Duration: 30 * time.Second},
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, err := os.Open("./testdata/" + tt.name + ".yml")
			require.NoError(t, err, t.Name())

			defer func() {
				if err := r.Close(); err != nil {
					log.Fatalf("error closing file handler: %v", err)
				}
			}()

			cfg, errer := Parse(r)
			if tt.err {
				require.Error(t, errer)
				return
			}

			require.Equal(t, tt.expCfg, cfg)
		})
	}
}
