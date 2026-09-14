###############################################################################
# proxmox-vm — виходи.
#
# Призначення: дати середовищу й майбутньому генератору Ansible-інвентаря (M3)
# усе потрібне для адресації VM. configured_ipv4 — детермінований (з cloud-init),
# доступний одразу; ipv4_addresses — фактичний, з qemu-guest-agent (може бути
# порожнім на першому apply, поки агент не стартував).
###############################################################################

output "vmid" {
  description = "VMID створеної віртуальної машини."
  value       = proxmox_virtual_environment_vm.this.vm_id
}

output "name" {
  description = "Ім'я VM (hostname)."
  value       = proxmox_virtual_environment_vm.this.name
}

output "node" {
  description = "Вузол Proxmox, на якому розміщено VM."
  value       = proxmox_virtual_environment_vm.this.node_name
}

output "configured_ipv4" {
  description = "Заданий cloud-init IPv4 без маски (або 'dhcp'). Детермінований, для інвентаря M3."
  value       = var.ip_address == "dhcp" ? "dhcp" : split("/", var.ip_address)[0]
}

output "ipv4_addresses" {
  description = "Фактичні IPv4 per-interface від qemu-guest-agent (порожньо, якщо agent вимкнено/не стартував)."
  value       = proxmox_virtual_environment_vm.this.ipv4_addresses
}

output "mac_addresses" {
  description = "MAC-адреси інтерфейсів VM."
  value       = proxmox_virtual_environment_vm.this.mac_addresses
}
