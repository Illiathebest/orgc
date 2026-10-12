# iac-runner — єдине середовище виконання всіх CI-jobs (M1).
# База: python:3.12-slim (glibc → готові wheels для cryptography/paramiko/lxml;
# обґрунтування проти ubuntu/alpine — ADR-005, Додаток А).
# Збірка: ci/runner-image.gitlab-ci.yml через Kaniko (без docker-in-docker).
FROM python:3.12-slim-bookworm

# Версії бінарних інструментів (M1 pin set, 2026-06). Kaniko не заповнює
# TARGETARCH автоматично, тому задаємо amd64 явно (стенд pve-a = x86_64).
ARG OPENTOFU_VERSION=1.12.1
# Go для збірки й тестів orgc. Контрольна сума — з dl.google.com (файл .sha256
# поруч з архівом); архів, що не збігся, зупиняє збірку.
ARG GO_VERSION=1.27.1
ARG GO_SHA256=63d339f0da5ab53635a56f2490a7984dfe12dfcff22ad749f63edaf590168445
ARG TFLINT_VERSION=0.63.1
ARG TARGETARCH=amd64

ENV DEBIAN_FRONTEND=noninteractive \
    PIP_NO_CACHE_DIR=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    ANSIBLE_COLLECTIONS_PATH=/usr/share/ansible/collections \
    PYTHONUNBUFFERED=1 \
    PATH=/usr/local/go/bin:${PATH} \
    GOTOOLCHAIN=local \
    CGO_ENABLED=0

# --- Системні залежності (мінімум; openssh-client для network_cli/SSH) ---
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        git \
        openssh-client \
        unzip \
    && rm -rf /var/lib/apt/lists/*

# --- Python-тулінг (пінується у requirements-pip.txt) ---
COPY runner-image/requirements-pip.txt /tmp/requirements-pip.txt
RUN pip install -r /tmp/requirements-pip.txt \
    && rm -f /tmp/requirements-pip.txt

# --- Ansible-колекції з requirements.yml (системний шлях) ---
COPY ansible/requirements.yml /tmp/requirements.yml
RUN ansible-galaxy collection install -r /tmp/requirements.yml -p "${ANSIBLE_COLLECTIONS_PATH}" \
    && rm -f /tmp/requirements.yml

# --- OpenTofu (бінар з офіційного релізу) ---
RUN curl -fsSL -o /tmp/tofu.zip \
        "https://github.com/opentofu/opentofu/releases/download/v${OPENTOFU_VERSION}/tofu_${OPENTOFU_VERSION}_linux_${TARGETARCH}.zip" \
    && unzip -o -d /usr/local/bin /tmp/tofu.zip tofu \
    && chmod +x /usr/local/bin/tofu \
    && rm -f /tmp/tofu.zip

# --- tflint (бінар з офіційного релізу) ---
RUN curl -fsSL -o /tmp/tflint.zip \
        "https://github.com/terraform-linters/tflint/releases/download/v${TFLINT_VERSION}/tflint_linux_${TARGETARCH}.zip" \
    && unzip -o -d /usr/local/bin /tmp/tflint.zip tflint \
    && chmod +x /usr/local/bin/tflint \
    && rm -f /tmp/tflint.zip

# --- Go (офіційний архів, перевірений за SHA-256) ---
# GOTOOLCHAIN=local: go не завантажує інший toolchain, навіть якщо go.mod його
# попросить, — збірка лишається на версії, зафіксованій тут.
# CGO_ENABLED=0: C-компілятора в образі немає, orgc він не потрібен.
RUN curl -fsSL -o /tmp/go.tgz "https://dl.google.com/go/go${GO_VERSION}.linux-${TARGETARCH}.tar.gz" \
    && echo "${GO_SHA256}  /tmp/go.tgz" | sha256sum -c - \
    && tar -C /usr/local -xzf /tmp/go.tgz \
    && rm -f /tmp/go.tgz

# --- Перевірка вмісту під час збірки (fail-fast, якщо щось не стало) ---
RUN ansible --version \
    && ansible-lint --version \
    && yamllint --version \
    && check-jsonschema --version \
    && tofu version \
    && tflint --version \
    && go version \
    && python -c "import librouteros, napalm, napalm_ros; print('net libs OK')" \
    && ansible-galaxy collection list

WORKDIR /workspace
CMD ["bash"]
