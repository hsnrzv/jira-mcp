# Windows PowerShell 5.1 + WinForms. One persistent window for the whole setup
# wizard. Commands arrive as JSON lines on stdin ($JmcpIn is the reader left by
# the bootstrap); user actions are written as JSON lines to stdout. The window
# has the usual minimize/close buttons; closing it (or "Close server") emits
# {"action":"close"} and exits.
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
[System.Windows.Forms.Application]::EnableVisualStyles()

if (-not $JmcpIn) {
  $JmcpIn = New-Object System.IO.StreamReader([Console]::OpenStandardInput(), (New-Object System.Text.UTF8Encoding $false))
}
$script:out = New-Object System.IO.StreamWriter([Console]::OpenStandardOutput(), (New-Object System.Text.UTF8Encoding $false))
$script:out.AutoFlush = $true

$W = 460; $M = 20; $ListH = 220
$script:title = 'Jira MCP Setup'
$script:icon = $null; $script:iconImage = $null
$script:closing = $false
$script:shown = $false
$script:buttons = @()
$script:refs = @{}
$script:status = @{ text = ''; kind = '' }
$script:rows = @()
$script:pending = $null
$script:projects = @()
$script:checked = @{}
$script:filtering = $false

function Emit($obj) {
  $script:out.WriteLine((ConvertTo-Json -InputObject $obj -Compress -Depth 4))
}

function Close-Server {
  if ($script:closing) { return }
  $script:closing = $true
  try { Emit @{ action = 'close' } } catch { }
  $script:ctx.ExitThread()
}

function Quit {
  $script:closing = $true
  $script:ctx.ExitThread()
}

# ---------- window ----------

$script:form = New-Object System.Windows.Forms.Form
$form = $script:form
$form.Text = $script:title
$form.FormBorderStyle = 'FixedSingle'
$form.MaximizeBox = $false
$form.MinimizeBox = $true
$form.ShowInTaskbar = $true
$form.StartPosition = 'CenterScreen'
$form.AutoScaleMode = 'Dpi'
$form.Font = New-Object System.Drawing.Font('Segoe UI', 9)
$form.ClientSize = New-Object System.Drawing.Size(($W + 2 * $M), 200)
$form.add_FormClosing({
  param($s, $e)
  try { if (-not $script:closing) { Close-Server } } catch { [Console]::Error.WriteLine($_) }
})
$script:ctx = New-Object System.Windows.Forms.ApplicationContext

function New-Label([string]$text, [bool]$bold = $false, $color = $null, [int]$width = $W) {
  $l = New-Object System.Windows.Forms.Label
  $l.Text = $text
  $l.AutoSize = $false
  $l.Width = $width
  if ($bold) { $l.Font = New-Object System.Drawing.Font($form.Font, [System.Drawing.FontStyle]::Bold) }
  if ($color) { $l.ForeColor = $color }
  $size = [System.Windows.Forms.TextRenderer]::MeasureText($text, $l.Font, (New-Object System.Drawing.Size($width, 0)), [System.Windows.Forms.TextFormatFlags]::WordBreak)
  $l.Height = [Math]::Max($size.Height + 2, 18)
  return $l
}

function New-Field([string]$value, [bool]$secret = $false) {
  $t = New-Object System.Windows.Forms.TextBox
  $t.Width = $W
  $t.Text = $value
  if ($secret) { $t.UseSystemPasswordChar = $true }
  return $t
}

function New-Button([string]$text, [scriptblock]$handler, [bool]$primary = $false, [int]$width = 0) {
  $b = New-Object System.Windows.Forms.Button
  $b.Text = $text
  $b.Height = 30
  $b.Width = [Math]::Max($width, [Math]::Max(96, [System.Windows.Forms.TextRenderer]::MeasureText($text, $form.Font).Width + 24))
  $b.add_Click($handler)
  if ($primary) { $form.AcceptButton = $b }
  $script:buttons += $b
  return $b
}

function New-CloseButton {
  $b = New-Object System.Windows.Forms.Button
  $b.Text = 'Close server'
  $b.Height = 30
  $b.Width = 110
  $b.FlatStyle = 'Flat'
  $b.FlatAppearance.BorderSize = 0
  $b.BackColor = [System.Drawing.Color]::FromArgb(220, 38, 38)
  $b.ForeColor = [System.Drawing.Color]::White
  $b.Font = New-Object System.Drawing.Font($form.Font, [System.Drawing.FontStyle]::Bold)
  $b.add_Click({ Close-Server })
  return $b
}

# Button row: "Close server" at the far left, other buttons right-aligned.
function New-ButtonRow($list) {
  $p = New-Object System.Windows.Forms.Panel
  $p.Width = $W; $p.Height = 30
  $c = New-CloseButton
  $c.Location = New-Object System.Drawing.Point(0, 0)
  $p.Controls.Add($c)
  $x = $W
  for ($i = $list.Count - 1; $i -ge 0; $i--) {
    $b = $list[$i]
    $x -= $b.Width
    $b.Location = New-Object System.Drawing.Point($x, 0)
    $p.Controls.Add($b)
    $x -= 8
  }
  return $p
}

function New-Header([string]$step) {
  $p = New-Object System.Windows.Forms.Panel
  $p.Width = $W; $p.Height = 48
  $x = 0
  if ($script:iconImage) {
    $pb = New-Object System.Windows.Forms.PictureBox
    $pb.Image = $script:iconImage
    $pb.SizeMode = 'Zoom'
    $pb.Size = New-Object System.Drawing.Size(48, 48)
    $p.Controls.Add($pb)
    $x = 60
  }
  $t = New-Object System.Windows.Forms.Label
  $t.Text = $script:title
  $t.Font = New-Object System.Drawing.Font('Segoe UI', 13, [System.Drawing.FontStyle]::Bold)
  $t.Location = New-Object System.Drawing.Point($x, 2)
  $t.Size = New-Object System.Drawing.Size(($W - $x), 26)
  $p.Controls.Add($t)
  $s = New-Object System.Windows.Forms.Label
  $s.Text = $step
  $s.ForeColor = [System.Drawing.SystemColors]::GrayText
  $s.Location = New-Object System.Drawing.Point($x, 28)
  $s.Size = New-Object System.Drawing.Size(($W - $x), 18)
  $p.Controls.Add($s)
  return $p
}

function Row($control, [int]$gap) { return @{ control = $control; gap = $gap } }
function TextRow([string]$text, [int]$gap, [bool]$bold = $false) { return Row (New-Label $text $bold) $gap }

function New-StatusPanel {
  $s = $script:status
  if (-not $s.text -and $s.kind -ne 'busy') { return $null }
  $p = New-Object System.Windows.Forms.Panel
  $p.Width = $W
  $p.BorderStyle = 'FixedSingle'
  $fg = $null
  switch ($s.kind) {
    'error' { $p.BackColor = [System.Drawing.Color]::MistyRose; $fg = [System.Drawing.Color]::FromArgb(185, 28, 28) }
    'ok'    { $p.BackColor = [System.Drawing.Color]::Honeydew;  $fg = [System.Drawing.Color]::FromArgb(21, 128, 61) }
  }
  $y = 8
  if ($s.kind -eq 'busy') {
    $bar = New-Object System.Windows.Forms.ProgressBar
    $bar.Style = 'Marquee'
    $bar.MarqueeAnimationSpeed = 30
    $bar.Location = New-Object System.Drawing.Point(8, $y)
    $bar.Size = New-Object System.Drawing.Size(($W - 18), 10)
    $p.Controls.Add($bar)
    $y += 16
  }
  if ($s.text) {
    $text = $s.text
    if ($s.kind -eq 'error') { $text = "$([char]0x26A0) $text" }
    $l = New-Label $text ($s.kind -eq 'error') $fg ($W - 18)
    $l.Location = New-Object System.Drawing.Point(8, $y)
    $p.Controls.Add($l)
    $y += $l.Height
  }
  $p.Height = $y + 8
  return $p
}

function Layout {
  $form.SuspendLayout()
  $form.Controls.Clear()
  $y = $M
  $rows = @()
  foreach ($r in $script:rows) {
    if ($r.status) {
      $sp = New-StatusPanel
      if ($sp) { $rows += (Row $sp 14) }
    } else { $rows += $r }
  }
  for ($i = 0; $i -lt $rows.Count; $i++) {
    $c = $rows[$i].control
    $c.Location = New-Object System.Drawing.Point($M, $y)
    $form.Controls.Add($c)
    $y += $c.Height
    if ($i -lt $rows.Count - 1) { $y += $rows[$i].gap }
  }
  $form.ClientSize = New-Object System.Drawing.Size(($W + 2 * $M), ($y + $M))
  $form.ResumeLayout()
}

function New-Page($rows, $focus) {
  $script:rows = $rows
  Layout
  if (-not $script:shown) {
    $script:shown = $true
    $form.Show()
    # Bring to front once without staying on top of other apps.
    $form.TopMost = $true; $form.TopMost = $false
    $form.Activate()
  }
  if ($focus) { [void]$focus.Focus() }
}

function Set-Status([string]$text, [string]$kind) {
  $script:status = @{ text = $text; kind = $kind }
  if ($kind -ne 'busy') { foreach ($b in $script:buttons) { $b.Enabled = $true } }
  Layout
  if ($kind -eq 'error') { [System.Media.SystemSounds]::Exclamation.Play() }
}

function Set-Busy([string]$text) {
  foreach ($b in $script:buttons) { $b.Enabled = $false }
  Set-Status $text 'busy'
}

function Reset-Page {
  $script:buttons = @()
  $script:refs = @{}
  $form.AcceptButton = $null
}

function Initial-Status($c) {
  if ($c.status) {
    $kind = 'ok'; if ($c.error) { $kind = 'error' }
    return @{ text = [string]$c.status; kind = $kind }
  }
  return @{ text = ''; kind = '' }
}

# ---------- pages ----------

function Show-Connect($c) {
  Reset-Page
  $url = New-Field ([string]$c.jiraUrl)
  $tok = New-Field '' $true
  $ca = New-Field ([string]$c.customCaCertPath)
  $hint = 'Paste your PAT'; if ($c.tokenHint) { $hint = [string]$c.tokenHint }
  $script:refs = @{ url = $url; tok = $tok; ca = $ca }
  $next = New-Button 'Next' {
    Set-Busy ('Connecting to Jira and loading your projects' + [char]0x2026)
    Emit @{ action = 'next'; jiraUrl = $script:refs.url.Text; personalToken = $script:refs.tok.Text; customCaCertPath = $script:refs.ca.Text }
  } $true
  $script:status = Initial-Status $c
  $focus = $url; if ($c.jiraUrl) { $focus = $tok }
  New-Page @(
    (Row (New-Header ('Step 1 of 2 ' + [char]0x00B7 + ' Connect')) 14),
    (TextRow ([string]$c.message) 14),
    (TextRow 'Jira URL' 4 $true), (Row $url 12),
    (TextRow ('Personal Access Token (' + $hint + ')') 4 $true), (Row $tok 12),
    (TextRow 'Corporate root CA file (PEM path, optional)' 4 $true), (Row $ca 18),
    @{ status = $true },
    (Row (New-ButtonRow @($next)) 0)
  ) $focus
}

function Fill-Projects([string]$filter) {
  $list = $script:refs.list
  $script:filtering = $true
  $list.BeginUpdate()
  $list.Items.Clear()
  $q = $filter.Trim().ToLowerInvariant()
  foreach ($p in $script:projects) {
    if ($q -and -not $p.search.Contains($q)) { continue }
    $i = $list.Items.Add($p.label)
    if ($script:checked[$p.key]) { $list.SetItemChecked($i, $true) }
  }
  $list.EndUpdate()
  $script:filtering = $false
}

function Selected-Keys {
  $keys = @()
  foreach ($p in $script:projects) { if ($script:checked[$p.key]) { $keys += $p.key } }
  return ,$keys
}

function Show-Projects($c) {
  Reset-Page
  $script:projects = @()
  $script:checked = @{}
  foreach ($s in @($c.selected)) { if ($s) { $script:checked[[string]$s] = $true } }
  $script:byLabel = @{}
  foreach ($p in @($c.projects)) {
    if (-not $p) { continue }
    $label = [string]$p.key
    if ($p.name) { $label = $label + ' ' + [char]0x2014 + ' ' + $p.name }
    $item = @{ key = [string]$p.key; label = $label; search = ($p.key + ' ' + $p.name).ToLowerInvariant() }
    $script:projects += $item
    $script:byLabel[$label] = $item.key
  }
  $search = New-Field ''
  $list = New-Object System.Windows.Forms.CheckedListBox
  $list.CheckOnClick = $true
  $list.IntegralHeight = $false
  $list.Width = $W; $list.Height = $ListH
  $list.add_ItemCheck({
    param($s, $e)
    if ($script:filtering) { return }
    $key = $script:byLabel[[string]$s.Items[$e.Index]]
    $script:checked[$key] = ($e.NewValue -eq [System.Windows.Forms.CheckState]::Checked)
  })
  $write = New-Object System.Windows.Forms.CheckBox
  $write.Text = 'Allow write tools (create/update issues, comments, transitions)'
  $write.Width = $W; $write.Height = 22
  $write.Checked = [bool]$c.enableWriteTools
  $script:refs = @{ search = $search; list = $list; write = $write }
  $search.add_TextChanged({ Fill-Projects $script:refs.search.Text })
  Fill-Projects ''
  $back = New-Button 'Back' {
    Set-Busy ''
    Emit @{ action = 'back'; projectKeys = (Selected-Keys); enableWriteTools = $script:refs.write.Checked }
  }
  $save = New-Button 'Save' {
    Set-Busy ('Checking the selected projects and saving' + [char]0x2026)
    Emit @{ action = 'save'; projectKeys = (Selected-Keys); enableWriteTools = $script:refs.write.Checked }
  } $true
  $script:status = @{ text = ''; kind = '' }
  New-Page @(
    (Row (New-Header ('Step 2 of 2 ' + [char]0x00B7 + ' Projects')) 14),
    (TextRow ([string]$c.message) 14),
    (TextRow ('Allowed project(s) ' + [char]0x2014 + ' JIRA_ALLOWED_PROJECTS (type to filter)') 4 $true),
    (Row $search 6),
    (Row $list 12),
    (Row $write 18),
    @{ status = $true },
    (Row (New-ButtonRow @($back, $save)) 0)
  ) $search
}

function Show-Register($c) {
  Reset-Page
  $rows = @(
    (Row (New-Header ('Setup complete ' + [char]0x00B7 + ' Register with your IDE')) 14),
    (TextRow ([string]$c.message) 14)
  )
  $i = 0
  foreach ($t in @($c.buttons)) {
    if (-not $t) { continue }
    $b = New-Button ([string]$t) {
      param($s, $e)
      Set-Busy ('Working' + [char]0x2026)
      Emit @{ action = 'choose'; index = [int]$s.Tag }
    } $false $W
    $b.Tag = $i
    $b.TextAlign = 'MiddleLeft'
    $rows += (Row $b 6)
    $i++
  }
  if ($i -gt 0) { $rows[$rows.Count - 1].gap = 18 }
  $script:status = Initial-Status $c
  $rows += @{ status = $true }
  $rows += (Row (New-ButtonRow @()) 0)
  New-Page $rows $null
}

function Copy-Text($c) {
  try {
    [System.Windows.Forms.Clipboard]::SetText([string]$c.text)
    Set-Status ([string]$c.message) 'ok'
  } catch {
    Set-Status 'Could not copy to the clipboard.' 'error'
  }
}

function Load-Icon($c) {
  if (-not $c.icon) { return }
  try {
    $bytes = [Convert]::FromBase64String([string]$c.icon)
    $ms = New-Object System.IO.MemoryStream(, $bytes)
    $bmp = New-Object System.Drawing.Bitmap($ms)
    $script:iconImage = $bmp
    $form.Icon = [System.Drawing.Icon]::FromHandle($bmp.GetHicon())
  } catch { }
}

function Handle($c) {
  switch ([string]$c.cmd) {
    'init' {
      if ($c.title) { $script:title = [string]$c.title }
      $form.Text = $script:title
      Load-Icon $c
    }
    'connect'  { Show-Connect $c }
    'projects' { Show-Projects $c }
    'register' { Show-Register $c }
    'status'   { $k = 'ok'; if ($c.error) { $k = 'error' }; Set-Status ([string]$c.status) $k }
    'copy'     { Copy-Text $c }
    'quit'     { Quit }
    default    { [Console]::Error.WriteLine('jira-mcp window: unknown command ' + $c.cmd) }
  }
}

# Poll stdin without blocking the UI thread.
$timer = New-Object System.Windows.Forms.Timer
$timer.Interval = 50
$timer.add_Tick({
  try {
    while ($true) {
      if (-not $script:pending) { $script:pending = $JmcpIn.ReadLineAsync() }
      if (-not $script:pending.IsCompleted) { return }
      $line = $script:pending.Result
      $script:pending = $null
      if ($null -eq $line) { Quit; return }   # parent went away
      $line = $line.Trim()
      if (-not $line) { continue }
      Handle (ConvertFrom-Json $line)
      if ($script:closing) { return }
    }
  } catch {
    [Console]::Error.WriteLine('jira-mcp window: ' + $_)
  }
})
$timer.Start()
[System.Windows.Forms.Application]::Run($script:ctx)
$timer.Stop()
$form.Dispose()
