$f, $panel = New-Dialog
Add-Label $panel $env:JMCP_MESSAGE
$row = New-ButtonRow $panel
$f.Tag = -1
$i = 0
foreach ($text in ($env:JMCP_BUTTONS -split "`n")) {
    if (-not $text) { continue }
    $b = Add-Button $row $text
    $b.Tag = $i
    $b.Add_Click({ param($s, $e) $form = $s.FindForm(); $form.Tag = [int]$s.Tag; $form.Close() })
    if ($i -eq 0) { $f.AcceptButton = $b }
    $i++
}
[void]$f.ShowDialog()
[Console]::Out.Write([string]$f.Tag)
