package openvpnweb

import (
	"encoding/json"
	"testing"
)

func TestMarshalForClientRedactsDashboardOnlineDetails(t *testing.T) {
	payload := DashboardStatsPayload{
		Summary:  DashboardSummary{Stats: DashboardStats{OnlineClients: 1}},
		Online:   []ClientData{{CommonName: "private-client"}},
		PushedAt: 123,
	}

	data, err := marshalForClient(&WsClient{permissions: map[string]bool{"menu:overview": true}}, wsBroadcast{envType: dashboardStatsTopic, payload: payload})
	if err != nil {
		t.Fatalf("marshal redacted dashboard payload: %v", err)
	}
	var envelope WsEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	body, ok := envelope.Payload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected object payload, got %T", envelope.Payload)
	}
	if online, exists := body["online"]; exists && online != nil {
		t.Fatalf("expected online details to be redacted, got %#v", online)
	}
}

func TestMarshalForClientDropsDashboardWithoutOverviewPermission(t *testing.T) {
	data, err := marshalForClient(&WsClient{permissions: map[string]bool{}}, wsBroadcast{envType: dashboardStatsTopic, payload: DashboardStatsPayload{}})
	if err != nil {
		t.Fatalf("marshal filtered dashboard payload: %v", err)
	}
	if data != nil {
		t.Fatalf("expected dashboard payload to be filtered, got %s", data)
	}
}

func TestMarshalForClientKeepsDashboardDetailsWithOnlinePermission(t *testing.T) {
	payload := DashboardStatsPayload{Online: []ClientData{{CommonName: "visible-client"}}}
	data, err := marshalForClient(&WsClient{permissions: map[string]bool{"menu:overview": true, "client:view_online": true}}, wsBroadcast{envType: dashboardStatsTopic, payload: payload})
	if err != nil {
		t.Fatalf("marshal visible dashboard payload: %v", err)
	}
	if len(data) == 0 || !containsBytes(data, []byte("visible-client")) {
		t.Fatalf("expected online client details in payload: %s", data)
	}
}

func containsBytes(value, needle []byte) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(value); i++ {
		match := true
		for j := range needle {
			if value[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
