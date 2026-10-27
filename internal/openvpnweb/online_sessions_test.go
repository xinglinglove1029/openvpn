package openvpnweb

import (
	"strings"
	"testing"
)

func TestOnlineSessionKeepsIdleManagementClient(t *testing.T) {
	// Last Ref records routed data use, not the session's keepalive. An idle
	// authenticated client remains online while management lists its session.
	response := strings.Join([]string{
		"TITLE\tOpenVPN test",
		"CLIENT_LIST\trocky\t203.0.113.4:9876\t10.8.0.3\t\t123\t456\t2026-01-01 00:00:00\t0\trocky\t7\t0",
		"ROUTING_TABLE\t10.8.0.3\trocky\t203.0.113.4:9876\t2026-01-01 00:00:00\t0",
		"END", "",
	}, "\r\n")
	clients := startManagementTestServer(t, response).getClient()
	if len(clients) != 1 || clients[0].ID != "7" {
		t.Fatalf("idle management session was incorrectly hidden: %+v", clients)
	}
}

func TestOnlineSessionDisappearsWhenManagementRemovesClient(t *testing.T) {
	response := "TITLE\tOpenVPN test\r\nROUTING_TABLE\t10.8.0.3\trocky\t203.0.113.4:9876\t2026-01-01 00:00:00\t0\r\nEND\r\n"
	if clients := startManagementTestServer(t, response).getClient(); len(clients) != 0 {
		t.Fatalf("routing entry alone created an online session: %+v", clients)
	}
}
