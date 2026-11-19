$f, $panel = New-Dialog
Add-Label $panel $env:JMCP_MESSAGE

function Add-TextBox([string]$label, [string]$value, [switch]$Password) {
    Add-Label $panel $label -Bold
    $t = New-Object System.Windows.Forms.TextBox
    $t.Width = $Width
    $t.Text = $value
    if ($Password) { $t.UseSystemPasswordChar = $true }
    $panel.Controls.Add($t)
    return $t
}

$tokenLabel = 'Personal Access Token'
if ($env:JMCP_TOKEN_HINT) { $tokenLabel = "$tokenLabel ($($env:JMCP_TOKEN_HINT))" }

$url = Add-TextBox 'Jira URL (e.g. https://jira.company.com)' $env:JMCP_URL
$tok = Add-TextBox $tokenLabel '' -Password
$keys = Add-TextBox 'Project key(s), comma-separated (e.g. PLAT,CORE)' $env:JMCP_KEYS
$write = New-Object System.Windows.Forms.CheckBox
$write.Text = 'Allow write tools (create/update issues, comments, transitions)'
$write.AutoSize = $true
$write.Margin = New-Object System.Windows.Forms.Padding(0, 10, 0, 0)
$write.Checked = ($env:JMCP_WRITE -eq '1')
$panel.Controls.Add($write)
$ca = Add-TextBox 'Corporate root CA file (PEM path, optional)' $env:JMCP_CA

$row = New-ButtonRow $panel
$save = Add-Button $row 'Save'
$save.DialogResult = [System.Windows.Forms.DialogResult]::OK
$test = Add-Button $row 'Test connection'
$test.DialogResult = [System.Windows.Forms.DialogResult]::Retry
$cancel = Add-Button $row 'Cancel'
$cancel.DialogResult = [System.Windows.Forms.DialogResult]::Cancel
$f.AcceptButton = $save
$f.CancelButton = $cancel
if ($env:JMCP_URL) { $f.ActiveControl = $tok } else { $f.ActiveControl = $url }

$action = switch ($f.ShowDialog().ToString()) {
    'OK' { 'save' }
    'Retry' { 'test' }
    default { 'cancel' }
}
$out = [ordered]@{
    action           = $action
    jiraUrl          = $url.Text
    personalToken    = $tok.Text
    projectKeys      = $keys.Text
    enableWriteTools = [bool]$write.Checked
    customCaCertPath = $ca.Text
}
[Console]::Out.Write(($out | ConvertTo-Json -Compress))
