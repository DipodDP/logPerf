<#
.SYNOPSIS
    Installs and configures OpenSSH Server on Windows.

.DESCRIPTION
    This script is intended to be run locally on a Windows machine as an Administrator.
    It performs the following actions:
    1. Installs the OpenSSH Server feature.
    2. Starts the sshd service and sets it to automatic startup.
    3. Configures the Windows Firewall to allow inbound TCP 22 for SSH.
    4. Prompts for an ED25519 public key and adds it to the appropriate authorized_keys file
       for secure passwordless login, respecting the Administrators group rule.

.NOTES
    Run this script in an elevated (Run as Administrator) PowerShell session.
#>

Write-Host "Starting OpenSSH Server Setup..." -ForegroundColor Cyan

# Check for Administrator privileges
if (-not ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Host "This script requires Administrator privileges. Attempting to elevate..." -ForegroundColor Yellow
    try {
        Start-Process powershell -ArgumentList "-ExecutionPolicy Bypass -NoProfile -File `"$PSCommandPath`"" -Verb RunAs
    } catch {
        Write-Host "Elevation failed or was cancelled. Please right-click PowerShell and select 'Run as Administrator', then run this script again." -ForegroundColor Red
    }
    exit
}

# 1. Enable OpenSSH Server
Write-Host "`n[1/4] Installing OpenSSH Server..." 
$sshState = Get-WindowsCapability -Online | Where-Object Name -like 'OpenSSH.Server*'
if ($sshState.State -eq 'Installed') {
    Write-Host "OpenSSH Server is already installed." -ForegroundColor Green
} else {
    Add-WindowsCapability -Online -Name OpenSSH.Server~~~~0.0.1.0 | Out-Null
    Write-Host "OpenSSH Server installed successfully." -ForegroundColor Green
}

# 2. Start and enable SSH service
Write-Host "`n[2/4] Configuring sshd service startup..."
Start-Service sshd
Set-Service -Name sshd -StartupType Automatic
Write-Host "Service 'sshd' started and set to Automatic." -ForegroundColor Green

# 3. Allow SSH and iperf3 in Windows Firewall
Write-Host "`n[3/4] Configuring Windows Firewall for TCP 22 and 5201..."
if (!(Get-NetFirewallRule -Name "sshd" -ErrorAction SilentlyContinue)) {
    New-NetFirewallRule -Name sshd -DisplayName 'OpenSSH Server (sshd)' -Enabled True -Direction In -Protocol TCP -Action Allow -LocalPort 22 | Out-Null
    Write-Host "SSH Firewall rule created." -ForegroundColor Green
} else {
    Write-Host "SSH Firewall rule already exists." -ForegroundColor Green
}

if (!(Get-NetFirewallRule -Name "iperf3" -ErrorAction SilentlyContinue)) {
    New-NetFirewallRule -Name iperf3 -DisplayName 'iperf3 Server (TCP)' -Enabled True -Direction In -Protocol TCP -Action Allow -LocalPort 5201 | Out-Null
    Write-Host "iperf3 TCP firewall rule created for port 5201." -ForegroundColor Green
} else {
    Write-Host "iperf3 TCP firewall rule already exists." -ForegroundColor Green
}

if (!(Get-NetFirewallRule -Name "iperf3-udp" -ErrorAction SilentlyContinue)) {
    New-NetFirewallRule -Name iperf3-udp -DisplayName 'iperf3 Server (UDP)' -Enabled True -Direction In -Protocol UDP -Action Allow -LocalPort 5201 | Out-Null
    Write-Host "iperf3 UDP firewall rule created for port 5201." -ForegroundColor Green
} else {
    Write-Host "iperf3 UDP firewall rule already exists." -ForegroundColor Green
}

# 4. Configure SSH Keys
Write-Host "`n[4/4] Configuring SSH Public Key Authentication..."
$pubKey = Read-Host "Paste your public key (e.g. ssh-ed25519 AAAA...) or press Enter to skip"

if (![string]::IsNullOrWhiteSpace($pubKey)) {
    # Check if user is Admin, OpenSSH on Windows requires admins to use a specific file
    $principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
    $isAdmin = $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)

    if ($isAdmin) {
        $sshDir = "$env:ProgramData\ssh"
        $authFile = "$sshDir\administrators_authorized_keys"
        
        Write-Host "Administrator account detected. Keys must be placed in $authFile"
        if (!(Test-Path $sshDir)) {
            New-Item -ItemType Directory -Force -Path $sshDir | Out-Null
        }
        
        Add-Content -Path $authFile -Value $pubKey
        
        # OpenSSH strictly enforces ACLs on administrators_authorized_keys
        Write-Host "Fixing permissions on administrators_authorized_keys..."
        icacls.exe $authFile /inheritance:r /grant "Administrators:F" /grant "SYSTEM:F" | Out-Null
        
        Write-Host "Public key added successfully for Administrator login." -ForegroundColor Green
    } else {
        $sshDir = "$env:USERPROFILE\.ssh"
        $authFile = "$sshDir\authorized_keys"
        
        if (!(Test-Path $sshDir)) {
            New-Item -ItemType Directory -Force -Path $sshDir | Out-Null
        }
        
        Add-Content -Path $authFile -Value $pubKey
        Write-Host "Public key added successfully to $authFile." -ForegroundColor Green
    }
}

# Restart SSHD to ensure all configs/paths take effect
Restart-Service sshd

Write-Host "`nSetup Complete! You can now SSH into this machine securely using your key." -ForegroundColor Cyan
