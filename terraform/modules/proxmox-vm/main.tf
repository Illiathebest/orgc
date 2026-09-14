###############################################################################
# proxmox-vm — створення VM клонуванням golden-шаблона + конфігурація cloud-init.
#
# Логіка:
#   * клон із шаблона (template_vmid) на цільовому вузлі (target_node);
#   * системний диск scsi0 на заданому сховищі, розмір — var.disk_gb;
#   * одна мережева карта на bridge (+ optional VLAN tag);
#   * cloud-init: статичний IP або DHCP, користувач + SSH-ключі, DNS;
#   * стан/IP повертаються через outputs.tf для збирання Ansible-інвентаря (M3).
#
# Примітки до вибору параметрів (production-ready, не заглушки):
#   * cpu.type за замовчуванням x86-64-v2-AES (переносність між вузлами, ADR-001);
#   * initialization.upgrade = var.ci_upgrade (false під API-токеном, ADR-007);
#   * stop_on_destroy прив'язано до agent_enabled — без агента graceful-shutdown
#     може зависати, тож тоді зупиняємо VM жорстко (рекомендація провайдера).
###############################################################################

locals {
  # Шлюз має бути відсутнім при DHCP (вимога провайдера/cloud-init).
  ipv4_gateway = var.ip_address == "dhcp" ? null : var.gateway

  # Чи задавати блок dns взагалі.
  dns_enabled = length(var.dns_servers) > 0 || var.dns_domain != null

  # Без агента graceful-shutdown ненадійний → зупиняємо жорстко на destroy.
  stop_on_destroy = !var.agent_enabled
}

resource "proxmox_virtual_environment_vm" "this" {
  name        = var.name
  description = var.description
  node_name   = var.target_node
  vm_id       = var.vm_id
  pool_id     = var.pool_id
  tags        = length(var.tags) > 0 ? sort(var.tags) : null

  started         = var.started
  on_boot         = var.on_boot
  stop_on_destroy = local.stop_on_destroy

  agent {
    enabled = var.agent_enabled
  }

  operating_system {
    type = "l26" # Linux 2.6 - 6.x (Ubuntu 24.04)
  }

  clone {
    vm_id        = var.template_vmid
    full         = var.full_clone
    datastore_id = var.datastore_id
  }

  cpu {
    cores = var.vcpu
    type  = var.cpu_type
  }

  memory {
    dedicated = var.memory_mb
    floating  = var.memory_floating_mb
  }

  disk {
    datastore_id = var.datastore_id
    interface    = "scsi0"
    size         = var.disk_gb
    discard      = var.disk_discard
    ssd          = false # SSD-емуляція несумісна з VirtIO Block; для scsi не потрібна
  }

  network_device {
    bridge  = var.network_bridge
    model   = var.network_model
    vlan_id = var.vlan_id
  }

  initialization {
    datastore_id = var.datastore_id
    upgrade      = var.ci_upgrade

    ip_config {
      ipv4 {
        address = var.ip_address
        gateway = local.ipv4_gateway
      }
    }

    dynamic "dns" {
      for_each = local.dns_enabled ? [1] : []
      content {
        domain  = var.dns_domain
        servers = length(var.dns_servers) > 0 ? var.dns_servers : null
      }
    }

    user_account {
      username = var.ci_user
      keys     = var.ssh_public_keys
    }
  }

  lifecycle {
    # Якщо статичну адресу задано — шлюз обов'язковий (cloud-init без шлюзу
    # дасть VM без маршруту за межі підмережі).
    precondition {
      condition     = var.ip_address == "dhcp" || var.gateway != null
      error_message = "Для статичного ip_address (${var.ip_address}) треба задати gateway."
    }
    # Доступ без пароля: при увімкненому агенті без ключів VM буде недосяжною
    # для Ansible на Day-1 — попереджаємо ще на plan.
    precondition {
      condition     = length(var.ssh_public_keys) > 0
      error_message = "ssh_public_keys порожній — VM не матиме способу входу для Ansible (M3). Додайте хоча б один ключ."
    }
  }
}
