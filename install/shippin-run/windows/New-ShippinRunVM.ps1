# Create a Hyper-V virtual machine for the Shippin Run control plane.
# Requires an elevated PowerShell on the Windows host, an Ubuntu ISO, and a virtual switch.
param(
  [Parameter(Mandatory = $true)][string]$VMName,
  [Parameter(Mandatory = $true)][string]$IsoPath,
  [Parameter(Mandatory = $true)][string]$SwitchName,
  [int]$MemoryGB = 4,
  [int]$VCPU = 2,
  [int]$DiskGB = 40
)

$ErrorActionPreference = "Stop"
if (-not (Get-Command New-VM -ErrorAction SilentlyContinue)) {
  throw "Hyper-V PowerShell tools are not available. Install the Hyper-V role and run this in an elevated session."
}
if (-not (Test-Path -LiteralPath $IsoPath)) {
  throw "ISO not found: $IsoPath"
}
$vhd = Join-Path $env:PUBLIC ("Documents\Hyper-V\Virtual hard disks\" + $VMName + ".vhdx")
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $vhd) | Out-Null
New-VM -Name $VMName -Generation 2 -MemoryStartupBytes ($MemoryGB * 1GB) -SwitchName $SwitchName -NewVHDPath $vhd -NewVHDSizeBytes ($DiskGB * 1GB) | Out-Null
Set-VMProcessor -VMName $VMName -Count $VCPU
Set-VMFirmware -VMName $VMName -EnableSecureBoot On -SecureBootTemplate "MicrosoftUEFICertificateAuthority"
Add-VMDvdDrive -VMName $VMName -Path $IsoPath
$dvd = Get-VMDvdDrive -VMName $VMName
Set-VMFirmware -VMName $VMName -FirstBootDevice $dvd
Write-Output "Created $VMName. Start it with: Start-VM -Name $VMName"
Write-Output "Finish the Ubuntu install from the Hyper-V console, then run install/shippin-run/deploy-control-plane.sh inside the guest."
