# Windows PowerShell 5.1+ (WinForms). Inputs arrive only via JMCP_* environment
# variables; the result is written to stdout.
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
[System.Windows.Forms.Application]::EnableVisualStyles()

$Width = 440

function New-Dialog {
    $f = New-Object System.Windows.Forms.Form
    $f.Text = $env:JMCP_TITLE
    $f.FormBorderStyle = [System.Windows.Forms.FormBorderStyle]::FixedDialog
    $f.MaximizeBox = $false
    $f.MinimizeBox = $false
    $f.ShowInTaskbar = $true
    $f.TopMost = $true
    $f.StartPosition = [System.Windows.Forms.FormStartPosition]::CenterScreen
    $f.AutoSize = $true
    $f.AutoSizeMode = [System.Windows.Forms.AutoSizeMode]::GrowAndShrink
    $f.Padding = New-Object System.Windows.Forms.Padding(14)
    $f.Font = [System.Drawing.SystemFonts]::MessageBoxFont
    if ($env:JMCP_ICON_PNG) {
        try {
            $ms = New-Object System.IO.MemoryStream(, [Convert]::FromBase64String($env:JMCP_ICON_PNG))
            $bmp = New-Object System.Drawing.Bitmap($ms)
            $f.Icon = [System.Drawing.Icon]::FromHandle($bmp.GetHicon())
        } catch { }
    }
    $f.Add_Shown({ param($s, $e) $s.Activate() })

    $panel = New-Object System.Windows.Forms.FlowLayoutPanel
    $panel.FlowDirection = [System.Windows.Forms.FlowDirection]::TopDown
    $panel.WrapContents = $false
    $panel.AutoSize = $true
    $panel.AutoSizeMode = [System.Windows.Forms.AutoSizeMode]::GrowAndShrink
    $f.Controls.Add($panel)
    return $f, $panel
}

function Add-Label($panel, [string]$text, [switch]$Bold) {
    $l = New-Object System.Windows.Forms.Label
    $l.Text = $text
    $l.AutoSize = $true
    $l.MaximumSize = New-Object System.Drawing.Size($Width, 0)
    $l.Margin = New-Object System.Windows.Forms.Padding(0, 8, 0, 2)
    if ($Bold) { $l.Font = New-Object System.Drawing.Font($l.Font, [System.Drawing.FontStyle]::Bold) }
    $panel.Controls.Add($l)
}

function New-ButtonRow($panel) {
    $row = New-Object System.Windows.Forms.FlowLayoutPanel
    $row.FlowDirection = [System.Windows.Forms.FlowDirection]::LeftToRight
    $row.WrapContents = $true
    $row.AutoSize = $true
    $row.AutoSizeMode = [System.Windows.Forms.AutoSizeMode]::GrowAndShrink
    $row.MaximumSize = New-Object System.Drawing.Size($Width, 0)
    $row.Margin = New-Object System.Windows.Forms.Padding(0, 16, 0, 0)
    $panel.Controls.Add($row)
    return $row
}

function Add-Button($row, [string]$text) {
    $b = New-Object System.Windows.Forms.Button
    $b.Text = $text
    $b.AutoSize = $true
    $b.MinimumSize = New-Object System.Drawing.Size(88, 0)
    $b.Padding = New-Object System.Windows.Forms.Padding(6, 2, 6, 2)
    $row.Controls.Add($b)
    return $b
}
