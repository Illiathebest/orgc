#!/usr/bin/env bash
# Джерело пароля Ansible Vault:
#   1. CI:      protected variable ANSIBLE_VAULT_PASSWORD (GitLab CI/CD settings)
#   2. локально: файл ~/.config/iac-infra/vault-pass (chmod 600, поза репозиторієм)
set -euo pipefail

if [[ -n "${ANSIBLE_VAULT_PASSWORD:-}" ]]; then
  printf '%s' "${ANSIBLE_VAULT_PASSWORD}"
elif [[ -f "${HOME}/.config/iac-infra/vault-pass" ]]; then
  cat "${HOME}/.config/iac-infra/vault-pass"
else
  echo "ERROR: vault password not found." >&2
  echo "Set ANSIBLE_VAULT_PASSWORD env var (CI) or create ~/.config/iac-infra/vault-pass (local)." >&2
  exit 1
fi
