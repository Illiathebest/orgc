###############################################################################
# proxmox-vm — вхідні змінні.
#
# Контракт модуля: середовище передає лише дані (ім'я, ресурси, мережа,
# cloud-init, цільовий вузол, шаблон-джерело). Жодних секретів тут немає —
# автентифікація до Proxmox конфігурується у провайдері середовища (ADR-007).
###############################################################################

# --- Ідентифікація та розміщення --------------------------------------------

variable "name" {
  description = "Ім'я VM (валідне DNS-ім'я: a-z, 0-9, дефіс). Стане hostname через cloud-init."
  type        = string

  validation {
    condition     = can(regex("^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$", var.name))
    error_message = "name має бути валідним DNS-іменем (малі літери, цифри, дефіс; не починатися/закінчуватися дефісом)."
  }
}

variable "vm_id" {
  description = "Фіксований VMID. null → Proxmox призначить наступний вільний (>= 100)."
  type        = number
  default     = null

  validation {
    condition     = var.vm_id == null || (var.vm_id >= 100 && var.vm_id <= 999999999)
    error_message = "vm_id має бути null або числом у діапазоні 100..999999999."
  }
}

variable "target_node" {
  description = "Ім'я вузла Proxmox, на якому створюється VM (наприклад pve-a або pve-b)."
  type        = string
}

variable "template_vmid" {
  description = "VMID шаблона-джерела для клонування (golden image, наприклад 9000 — ubuntu2404-tpl)."
  type        = number

  validation {
    condition     = var.template_vmid >= 100
    error_message = "template_vmid має бути >= 100."
  }
}

variable "full_clone" {
  description = "true → повний (незалежний) клон; false → linked clone (швидше, але залежить від шаблона)."
  type        = bool
  default     = true
}

variable "pool_id" {
  description = "Необов'язковий пул ресурсів Proxmox для VM. null → без пулу."
  type        = string
  default     = null
}

# --- Обчислювальні ресурси ----------------------------------------------------

variable "vcpu" {
  description = "Кількість vCPU (cores)."
  type        = number
  default     = 1

  validation {
    condition     = var.vcpu >= 1 && var.vcpu <= 128
    error_message = "vcpu має бути в діапазоні 1..128."
  }
}

variable "cpu_type" {
  description = <<-EOT
    Тип емульованого CPU. За замовчуванням 'x86-64-v2-AES' — рекомендація bpg/proxmox:
    стабільний набір інструкцій, переносний між вузлами (на відміну від 'host', який
    прив'язує VM до конкретної моделі CPU й ламає міграцію між pve-a/pve-b).
  EOT
  type        = string
  default     = "x86-64-v2-AES"
}

variable "memory_mb" {
  description = "Виділена пам'ять (dedicated), МБ."
  type        = number
  default     = 1024

  validation {
    condition     = var.memory_mb >= 128
    error_message = "memory_mb має бути >= 128."
  }
}

variable "memory_floating_mb" {
  description = <<-EOT
    Floating-пам'ять (балунінг), МБ. 0 → балунінг вимкнено (фіксована пам'ять).
    Щоб увімкнути балунінг, задайте значення, рівне memory_mb.
  EOT
  type        = number
  default     = 0

  validation {
    condition     = var.memory_floating_mb == 0 || var.memory_floating_mb <= var.memory_mb
    error_message = "memory_floating_mb має бути 0 або не більшим за memory_mb."
  }
}

# --- Диск ---------------------------------------------------------------------

variable "disk_gb" {
  description = "Розмір системного диска (scsi0), ГБ. Має бути >= розміру диска шаблона."
  type        = number
  default     = 20

  validation {
    condition     = var.disk_gb >= 1
    error_message = "disk_gb має бути >= 1."
  }
}

variable "datastore_id" {
  description = "Сховище Proxmox для диска та cloud-init (наприклад local-lvm)."
  type        = string
  default     = "local-lvm"
}

variable "disk_discard" {
  description = "Передавати discard/trim у сховище: 'on' (для thin/SSD) або 'ignore'."
  type        = string
  default     = "on"

  validation {
    condition     = contains(["on", "ignore"], var.disk_discard)
    error_message = "disk_discard має бути 'on' або 'ignore'."
  }
}

# --- Мережа -------------------------------------------------------------------

variable "network_bridge" {
  description = "Linux bridge на вузлі (наприклад vmbr0 — LAN, vmbr1 — lab)."
  type        = string
  default     = "vmbr0"
}

variable "vlan_id" {
  description = "VLAN tag для інтерфейсу. null → без тегування (access до native VLAN бриджа)."
  type        = number
  default     = null

  validation {
    condition     = var.vlan_id == null || (var.vlan_id >= 1 && var.vlan_id <= 4094)
    error_message = "vlan_id має бути null або в діапазоні 1..4094."
  }
}

variable "network_model" {
  description = "Модель віртуальної мережевої карти."
  type        = string
  default     = "virtio"

  validation {
    condition     = contains(["virtio", "e1000", "e1000e", "rtl8139", "vmxnet3"], var.network_model)
    error_message = "network_model має бути одним із: virtio, e1000, e1000e, rtl8139, vmxnet3."
  }
}

# --- cloud-init ---------------------------------------------------------------

variable "ip_address" {
  description = "IPv4 у CIDR (наприклад 192.168.1.61/24) або 'dhcp' для автоконфігурації."
  type        = string
  default     = "dhcp"

  validation {
    condition     = var.ip_address == "dhcp" || can(cidrhost(var.ip_address, 0))
    error_message = "ip_address має бути 'dhcp' або адресою в CIDR-нотації (напр. 192.168.1.61/24)."
  }
}

variable "gateway" {
  description = "IPv4-шлюз. Обов'язковий при статичній адресі; ігнорується при 'dhcp'."
  type        = string
  default     = null

  validation {
    condition     = var.gateway == null || can(regex("^(\\d{1,3}\\.){3}\\d{1,3}$", var.gateway))
    error_message = "gateway має бути null або валідною IPv4-адресою."
  }
}

variable "dns_servers" {
  description = "Список DNS-серверів для cloud-init. Порожній → налаштування вузла за замовчуванням."
  type        = list(string)
  default     = []
}

variable "dns_domain" {
  description = "DNS search domain для cloud-init. null → не задавати."
  type        = string
  default     = null
}

variable "ci_user" {
  description = "Ім'я користувача, що створюється cloud-init."
  type        = string
  default     = "ubuntu"
}

variable "ssh_public_keys" {
  description = "Публічні SSH-ключі для ci_user (вхід без пароля). Рекомендований спосіб доступу."
  type        = list(string)
  default     = []
}

variable "ci_upgrade" {
  description = <<-EOT
    Автоматичний apt-апгрейд після першого завантаження (cloud-init).
    ВАЖЛИВО: у bpg/proxmox ця опція дозволена лише для root@pam. Під API-токеном
    (наша модель — ADR-007) має бути false, інакше apply впаде з 403. Базову ОС
    оновлює Ansible на Day-1 (M3), тож тут false — правильний дефолт.
  EOT
  type        = bool
  default     = false
}

# --- Поведінка життєвого циклу ------------------------------------------------

variable "agent_enabled" {
  description = <<-EOT
    Очікувати qemu-guest-agent у гостьовій ОС. true → провайдер бачить реальні IP
    (вихід ipv4_addresses) і коректно вимикає/перезавантажує VM. Вимагає, щоб agent
    реально стартував у VM (cloud-init встановлює qemu-guest-agent — M3).
  EOT
  type        = bool
  default     = true
}

variable "started" {
  description = "Чи має VM бути запущеною після створення."
  type        = bool
  default     = true
}

variable "on_boot" {
  description = "Автостарт VM під час завантаження вузла."
  type        = bool
  default     = true
}

variable "tags" {
  description = "Теги VM (метаінформація). Зберігайте відсортованими, щоб уникнути дрифту."
  type        = list(string)
  default     = []
}

variable "description" {
  description = "Опис VM у Proxmox."
  type        = string
  default     = "Managed by OpenTofu (iac-infra)"
}
