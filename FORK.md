# Local CE 2.7 compatibility fixes

This fork is based on `emanuelbesliu/pfsense` v0.58.0 (commit `f16c920`).
It is not an official Netgate provider. See [PUBLISHING.md](PUBLISHING.md) for
signed GitHub releases and the required Terraform Registry registration.

## Changes

- Resolve firewall rule control IDs after `filter_rules_sort()`, matching the
  edit/delete GUI's ordering. Previously the raw configuration index could
  identify a different rule after the GUI sorted the list.
- Include the target rule's interface when deleting, so interface-specific
  separator handling uses the correct interface rather than the default WAN.
- Read a missing `ipprotocol` as `inet`, matching legacy wizard-generated
  IPv4 rules. Explicit user input remains validated.
- Resolve gateway GUI indices using the CE 2.7
  `return_gateways_array(true, false, true, true)` API when the newer indexed
  gateway API is unavailable. Guard `refresh_gateways()` and preserve actual
  PHP array keys instead of assuming slice positions are GUI indices.

## Build and test

Use Go 1.24.3 or newer, as required by the upstream module.

```sh
go test ./...
go test -race ./pkg/pfsense
go vet ./...
mkdir -p "$HOME/.local/share/pfsense-provider-fork"
go build -o "$HOME/.local/share/pfsense-provider-fork/terraform-provider-pfsense" .
```

Regression tests use a local
HTTP server to simulate sorted GUI indices, NAT-associated rules, missing
legacy address families, gateway keys, and missing resource identities. They
fail against unpatched upstream code. They do **not** execute PHP or replace
live CE 2.7 lifecycle tests.

## Use the local fork in an existing Terraform workspace

Keep `source = "emanuelbesliu/pfsense"` in Terraform for now. A CLI development
override substitutes this fork's executable without changing provider/state
addresses or requiring a Registry publication.

Save this as `$HOME/.config/terraform/pfsense-fork.tfrc`, substituting your real
absolute home directory in the path (create the parent directory first):

```hcl
provider_installation {
  dev_overrides {
    "emanuelbesliu/pfsense" = "/home/YOUR_USER/.local/share/pfsense-provider-fork"
  }
  direct {}
}
```

Then, from your platform workspace:

```sh
export TF_CLI_CONFIG_FILE="$HOME/.config/terraform/pfsense-fork.tfrc"
terraform validate
terraform plan
```

Terraform will warn that development overrides are active; this is expected.
The override bypasses normal version/checksum selection **for that provider**.
Do not rerun `terraform init` merely to install a development override. Other
providers still use normal installation. This configuration replaces any
other CLI configuration while the environment variable is set; merge existing
settings if needed. Run `unset TF_CLI_CONFIG_FILE` to stop selecting the fork.

The Go module path is intentionally unchanged to avoid unrelated import churn.
Publishing under `kthezelais/pfsense` uses the signed release workflow and
Registry setup described in [PUBLISHING.md](PUBLISHING.md). Do not change the
Terraform source until that registration is complete.

## Live-test precautions

- Keep a current configuration backup and independent LAN/console recovery.
- Capture the firewall rule trackers and gateway names before each test.
- Ensure no GUI user or other automation modifies rules during a test: the
  GUI API still uses numeric indices and is not transactional across requests.
- Keep `concurrent_writes = false` (the upstream default).
- Begin with a throwaway non-VPN rule, then verify that only its tracker is
  removed and that the VPN and management rules are unchanged.
- Test the non-default gateway separately, without touching WAN gateways.
- Reconcile previous partially failed resources before any whole-workspace
  destroy. Never delete Terraform state merely to hide a failed deletion.

These patches address the observed index and API compatibility failures. They
are not a guarantee that all upstream pfSense resources or apply/reload paths
are safe. Router-side lifecycle testing remains a separate, controlled step.
