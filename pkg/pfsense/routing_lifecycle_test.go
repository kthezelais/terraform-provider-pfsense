package pfsense

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// These tests exercise the HTTP client against a simulated GUI. They do not
// execute PHP or connect to a real firewall.
func lifecycleClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	endpoint, err := url.Parse(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	attempts, concurrent := 1, false
	return &Client{Options: &Options{URL: endpoint, MaxAttempts: &attempts, ConcurrentWrites: &concurrent}, httpClient: server.Client(), mutexes: &mutexes{}}
}

func lifecycleResponse(w http.ResponseWriter, value any) {
	data, _ := json.Marshal(value)
	fmt.Fprintf(w, "<html><head><title>Test</title></head><body><pre>%s</pre></body></html>", html.EscapeString(string(data)))
}

func TestParseFirewallRuleResponseAddressFamily(t *testing.T) {
	for _, family := range []string{"", "inet", "inet6", "inet46", "invalid"} {
		t.Run("family_"+family, func(t *testing.T) {
			rule, err := parseFirewallRuleResponse(firewallRuleResponse{Type: "pass", Interface: "openvpn", IPProtocol: family})
			if family == "invalid" {
				if !errors.Is(err, ErrClientValidation) {
					t.Fatalf("expected validation error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := family
			if want == "" {
				want = "inet"
			}
			if rule.IPProtocol != want {
				t.Fatalf("family = %q, want %q", rule.IPProtocol, want)
			}
		})
	}
	// Only reading legacy responses defaults to IPv4; user input stays strict.
	var rule FirewallRule
	if err := rule.SetIPProtocol(""); err == nil {
		t.Fatal("empty user-supplied address family must be rejected")
	}
}

func TestDeleteFirewallRuleUsesGUISortedIndex(t *testing.T) {
	// Raw config order differs from the GUI order. A NAT-associated rule also
	// occupies an index even though the provider does not manage it.
	vpn := firewallRuleResponse{Type: "pass", Interface: "openvpn", IPProtocol: "inet", Tracker: "vpn", Description: "Keep VPN access"}
	nat := firewallRuleResponse{Interface: "wan", AssociatedRuleID: "nat-association", Tracker: "nat"}
	target := firewallRuleResponse{Type: "pass", Interface: "lan", IPProtocol: "inet", Tracker: "target", Description: "Terraform test rule"}
	targetPresent, vpnPresent, deleteCalls := true, true, 0
	client := lifecycleClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			lifecycleResponse(w, nil)
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
			return
		}
		switch r.URL.Path {
		case "/diag_command.php":
			command := r.FormValue("txtPHPCommand")
			rules := []firewallRuleResponse{vpn, nat}
			if targetPresent {
				rules = append(rules, target)
			}
			if !strings.Contains(command, "filter_rules_sort();") && targetPresent {
				rules = []firewallRuleResponse{target, vpn, nat}
			}
			for i := range rules {
				rules[i].ControlID = i
			}
			lifecycleResponse(w, rules)
		case "/firewall_rules.php":
			deleteCalls++
			if r.FormValue("id") != "2" || r.FormValue("if") != "lan" {
				vpnPresent = false
				t.Errorf("unsafe delete request: id=%q interface=%q", r.FormValue("id"), r.FormValue("if"))
			} else {
				targetPresent = false
			}
			lifecycleResponse(w, nil)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	if err := client.DeleteFirewallRule(context.Background(), "target"); err != nil {
		t.Fatal(err)
	}
	if !vpnPresent || targetPresent || deleteCalls != 1 {
		t.Fatalf("VPN preserved=%v target remains=%v deletes=%d", vpnPresent, targetPresent, deleteCalls)
	}
}

func TestDeleteFirewallRuleMissingTrackerDoesNotDelete(t *testing.T) {
	client := lifecycleClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/firewall_rules.php" {
			t.Error("must not delete without a matching tracker")
		}
		lifecycleResponse(w, []firewallRuleResponse{})
	})
	if err := client.DeleteFirewallRule(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestGatewayGUIIndexCompatibility(t *testing.T) {
	for _, name := range []string{"TARGET", "MISSING"} {
		t.Run(name, func(t *testing.T) {
			client := lifecycleClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					lifecycleResponse(w, nil)
					return
				}
				if err := r.ParseForm(); err != nil {
					t.Error(err)
					return
				}
				command := r.FormValue("txtPHPCommand")
				for _, required := range []string{
					"function_exists('get_gateways') && defined('GW_CACHE_INDEXED')",
					"if (function_exists('refresh_gateways')) { refresh_gateways(); }",
					"return_gateways_array(true, false, true, true)",
					"'id' => (int)$k",
				} {
					if !strings.Contains(command, required) {
						t.Errorf("gateway lookup missing %q", required)
					}
				}
				// GUI indices need not match slice positions or raw config indices.
				lifecycleResponse(w, []map[string]any{{"id": 3, "name": "WAN"}, {"id": 7, "name": "TARGET"}})
			})
			id, err := client.getGatewayEditID(context.Background(), name)
			if name == "MISSING" {
				if !errors.Is(err, ErrNotFound) {
					t.Fatalf("expected not found, got %v", err)
				}
			} else if err != nil || id == nil || *id != 7 {
				t.Fatalf("expected GUI index 7, got %v, %v", id, err)
			}
		})
	}
}
