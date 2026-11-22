# Signed remote releases

This fork's intended Registry address is `kthezelais/pfsense`.
A GitHub release alone does not register a provider with Terraform.

## Release pipeline

Pushing a `v*` tag runs `.github/workflows/release.yml`:

1. Run unit tests and `go vet`.
2. Import the release-signing key from encrypted GitHub Actions secrets.
3. Build platform ZIP archives with GoReleaser.
4. Generate SHA256 checksums, sign them, and publish the assets to GitHub.

Release signing is configured using repository secrets `GPG_PRIVATE_KEY` and
`PASSPHRASE`. Never commit their values. The public key is
[`release-signing-key.asc`](release-signing-key.asc).

Public key fingerprint:

```text
CC43E5E0C0FE368C1525E739FCE3AF51AE79EC69
```

The key is RSA3072, signing-only, and expires two years after creation. Renew or
rotate it before expiry and update the Registry when necessary. Back up the
private key material securely; GitHub does not allow retrieving secret values.
The initial local recovery material was stored outside this repository at
`~/.local/share/pfsense-provider-release-signing/` with restricted permissions.
That directory is sensitive and must not be committed or shared.

Release Please is manual-only in this fork so pushes to main cannot
unexpectedly publish a version. Normal releases use an explicit version tag.

## One-time actions the GitHub account owner must complete

1. Sign into <https://registry.terraform.io> using the `kthezelais` GitHub
   account and grant the Registry access to this repository when prompted.
2. Open <https://registry.terraform.io/settings/gpg-keys> and add the entire
   contents of `release-signing-key.asc` as a signing key. This is the PUBLIC
   key, not either GitHub secret.
3. After the GitHub release has completed successfully, open
   <https://registry.terraform.io/publish/provider> and select
   `kthezelais/terraform-provider-pfsense`.
4. Confirm publication and wait until version `0.58.1` appears at
   <https://registry.terraform.io/providers/kthezelais/pfsense>.

The Registry installs its release webhook during registration. Future properly
signed GitHub releases should then be indexed automatically.

## Switch an existing Terraform configuration

Do this only AFTER version `0.58.1` is visible in the Registry. Back up your
Terraform state securely and confirm there is no other Terraform operation in
progress. No router changes are required merely to migrate the provider.

1. In the root module AND each child module using pfSense, change its
   `required_providers` entry to:

   ```hcl
   pfsense = {
     source  = "kthezelais/pfsense"
     version = "0.58.1"
   }
   ```

2. Disable any local development override for the old or new pfSense provider.
   If you used the dedicated configuration from FORK.md:

   ```sh
   unset TF_CLI_CONFIG_FILE
   ```

   Also check whether your normal Terraform CLI configuration contains an
   override. Provider connection settings and resource names stay unchanged.

3. If state still references the original provider, migrate that address:

   ```sh
   terraform state replace-provider \
     registry.terraform.io/emanuelbesliu/pfsense \
     registry.terraform.io/kthezelais/pfsense
   ```

   Review the prompt before confirming. This changes local state metadata,
   not router resources. Skip it if no resources use the original provider.

4. Install and inspect the fork without applying:

   ```sh
   terraform init
   terraform providers
   terraform validate
   terraform plan
   ```

5. Commit the updated dependency lock file. Do not immediately run a full
   workspace destroy: reconcile leftovers from previous partial failures,
   then test only disposable non-VPN resources with independent recovery.

Signing and Registry publication prove artifact integrity, not live router
compatibility. The fixes still require controlled CE 2.7 lifecycle testing.

Official instructions:
<https://developer.hashicorp.com/terraform/registry/providers/publishing>
