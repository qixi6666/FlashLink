package grpcapi

import "testing"

func TestRegisterServices(t *testing.T) {
	server := NewServer()

	RegisterLinkService(server, nil)
	RegisterRedirectService(server, nil)
	RegisterStatService(server, nil, nil)
}
