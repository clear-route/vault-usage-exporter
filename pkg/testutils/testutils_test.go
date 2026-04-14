//go:build integration

package testutils

import (
	"runtime"
	"testing"

	"github.com/clear-route/vault-usage-exporter/pkg/vault"
	"github.com/stretchr/testify/suite"
)

type vaultSuite struct {
	suite.Suite

	container *TestContainer
	client    *vault.Client
}

func (s *vaultSuite) TearDownSubTest() {
	s.Require().NoError(s.container.Terminate())
}

func (s *vaultSuite) SetupSubTest() {
	container, err := StartTestContainer()
	s.Require().NoError(err)

	client, err := vault.NewClientWithToken(container.URI, container.Token)
	s.Require().NoError(err)

	s.container = container
	s.client = client
}

func (s *vaultSuite) TestVaultConnection() {
	s.Run("health", func() {
		health, err := s.client.Sys().Health()
		s.Require().NoError(err)
		s.Require().True(health.Initialized, "initialized")
		s.Require().False(health.Sealed, "unsealed")
	})
}

func TestVaultSuite(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("docker-based integration tests are skipped on windows")
	}

	suite.Run(t, new(vaultSuite))
}
