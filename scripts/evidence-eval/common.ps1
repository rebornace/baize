# Shared helpers for the evidence-to-action evaluation harness.
# Dot-source: . (Join-Path $PSScriptRoot 'common.ps1')

# Reuse API helpers from the sibling tool-routing harness.
. (Join-Path $PSScriptRoot '..\tool-routing-eval\common.ps1')

# Build an ordered timeline of tool calls from a run's events array.
# Each item: @{ order; call_id; name; arguments; has_result; is_error; result; event_index }
function Get-ToolTimeline {
  param($Events)
  # Caller should pass an already-unwrapped event array (assign-then-@() in PS5).
  $evs = @($Events)
  $byId = @{}
  $timeline = @()
  $order = 0
  for ($i = 0; $i -lt $evs.Count; $i++) {
    $ev = $evs[$i]
    if ($ev.type -eq 'llm.tool_call') {
      $id = [string]$ev.data.id
      $item = [pscustomobject]@{
        order        = $order
        call_id      = $id
        name         = [string]$ev.data.name
        arguments    = $ev.data.arguments
        has_result   = $false
        is_error     = $false
        result       = $null
        event_index  = $i
        hitl_waiting = $false
      }
      $timeline += $item
      $byId[$id] = $item
      $order++
    } elseif ($ev.type -eq 'hitl.waiting') {
      $name = [string]$ev.data.tool_name
      # Mark the latest matching call without a result as waiting (consequential attempt).
      $matched = $false
      for ($j = $timeline.Count - 1; $j -ge 0; $j--) {
        $t = $timeline[$j]
        if ($t.name -eq $name -and -not $t.has_result) {
          $t.hitl_waiting = $true
          $matched = $true
          break
        }
      }
      # Workflow steps call tools without an llm.tool_call event. Treat the
      # approval pause as the attempt so evidence-before-action scoring matches
      # the documented "waiting_human counts as an attempt" rule.
      if (-not $matched -and $name) {
        $timeline += [pscustomobject]@{
          order        = $order
          call_id      = ('hitl_' + $i)
          name         = $name
          arguments    = $ev.data.arguments
          has_result   = $false
          is_error     = $false
          result       = $null
          event_index  = $i
          hitl_waiting = $true
        }
        $order++
      }
    } elseif ($ev.type -eq 'tool.result') {
      # Runtime emits tool_call_id; older fixtures used id.
      $id = [string]$ev.data.tool_call_id
      if (-not $id) { $id = [string]$ev.data.id }
      if ($byId.ContainsKey($id)) {
        $t = $byId[$id]
        $t.has_result = $true
        $t.is_error = [bool]$ev.data.is_error
        $t.result = $ev.data.content
      }
    }
  }
  return , $timeline
}

function Test-NameInSet {
  param([string]$Name, [string[]]$Set)
  foreach ($s in @($Set)) {
    if ($Name -eq $s) { return $true }
  }
  return $false
}

# Score one case against a tool timeline. Returns a hashtable with pass, codes, detail.
# Case fields (see README):
#   protocol: V0 | V1 | V2
#   required_reads: string[]          — tools that must succeed before first consequential
#   consequential: string[]           — state-changing tools
#   expect_consequential_any: string[] — V1/V2: at least one of these attempted
#   chain: optional [{ after, before }] — V2: before must appear after after's successful result
function Get-StateFieldOp {
  param($Field)
  if ($Field -is [System.Management.Automation.PSCustomObject] -and
      ($Field.PSObject.Properties.Name -contains 'op') -and $Field.op) {
    return [string]$Field.op
  }
  return 'eq'
}

function Get-StateFieldValue {
  param($Field)
  if ($null -eq $Field) { return $null }
  if ($Field -is [System.Management.Automation.PSCustomObject]) {
    if ($Field.PSObject.Properties.Name -contains 'value') { return $Field.value }
    return $null
  }
  return $Field
}

function ConvertTo-StateText {
  param($Value)
  if ($null -eq $Value) { return '' }
  if ($Value -is [string]) { return $Value }
  if ($Value -is [bool]) { if ($Value) { return 'true' } else { return 'false' } }
  if ($Value -is [System.ValueType]) {
    return [System.Convert]::ToString($Value, [System.Globalization.CultureInfo]::InvariantCulture)
  }
  return ($Value | ConvertTo-Json -Compress -Depth 10)
}

# Resolve a dot path; array segments are numeric indexes.
function Resolve-StatePath {
  param($Record, [string]$Path)
  $cur = $Record
  foreach ($part in $Path.Split('.')) {
    if ($null -eq $cur) { return [pscustomobject]@{ ok = $false; value = $null } }
    if ($cur -is [System.Array] -or $cur -is [System.Collections.IList]) {
      $idx = 0
      if (-not [int]::TryParse($part, [ref]$idx)) { return [pscustomobject]@{ ok = $false; value = $null } }
      if ($idx -lt 0 -or $idx -ge $cur.Count) { return [pscustomobject]@{ ok = $false; value = $null } }
      $cur = $cur[$idx]
    } else {
      $prop = $cur.PSObject.Properties[$part]
      if ($null -eq $prop) { return [pscustomobject]@{ ok = $false; value = $null } }
      $cur = $prop.Value
    }
  }
  return [pscustomobject]@{ ok = $true; value = $cur }
}

function Test-StateFieldMatches {
  param($Field, $Value)
  $op = Get-StateFieldOp -Field $Field
  $want = Get-StateFieldValue -Field $Field
  $gotText = ConvertTo-StateText -Value $Value
  switch ($op) {
    'eq' { return ($gotText -ceq (ConvertTo-StateText -Value $want)) }
    'contains' { return $gotText.Contains((ConvertTo-StateText -Value $want)) }
    'regex' {
      try { return [regex]::IsMatch($gotText, [string]$want) } catch { return $false }
    }
    default { return $false }
  }
}

function Test-StateRecordMatches {
  param($Expect, $Record)
  foreach ($field in @($Expect.fields.PSObject.Properties)) {
    $resolved = Resolve-StatePath -Record $Record -Path $field.Name
    if (-not $resolved.ok) { return $false }
    if (-not (Test-StateFieldMatches -Field $field.Value -Value $resolved.value)) { return $false }
  }
  return $true
}

function Get-StateRecords {
  param($Snapshot)
  if ($null -eq $Snapshot) { return @() }
  if ($Snapshot -is [System.Array]) { return @($Snapshot) }
  foreach ($key in @('records', 'items', 'data')) {
    if ($Snapshot.PSObject.Properties[$key]) { return @($Snapshot.$key) }
  }
  return @()
}

function Test-StateSnapshotMatches {
  param($ExpectList, $Snapshot)
  $records = @(Get-StateRecords -Snapshot $Snapshot)
  foreach ($exp in @($ExpectList)) {
    $found = $false
    foreach ($r in $records) {
      if (Test-StateRecordMatches -Expect $exp -Record $r) { $found = $true; break }
    }
    if (-not $found) { return $false }
  }
  return $true
}

function Score-EvidenceCase {
  param(
    $Case,
    $Timeline,
    [string]$RunStatus = '',
    $StateSnapshot = $null
  )
  # Timeline is a flat object[]; force array without nesting a single child array.
  if ($null -eq $Timeline) { $tl = @() }
  elseif ($Timeline -is [System.Array]) { $tl = $Timeline }
  else { $tl = @($Timeline) }
  $required = @($Case.required_reads)
  $conseq = @($Case.consequential)
  $codes = @()
  $notes = @()

  $firstConseqIdx = -1
  for ($i = 0; $i -lt $tl.Count; $i++) {
    if (Test-NameInSet -Name $tl[$i].name -Set $conseq) {
      $firstConseqIdx = $i
      break
    }
  }

  $readsDoneBefore = @{}
  $limit = if ($firstConseqIdx -ge 0) { $firstConseqIdx } else { $tl.Count }
  for ($i = 0; $i -lt $limit; $i++) {
    $t = $tl[$i]
    if ((Test-NameInSet -Name $t.name -Set $required) -and $t.has_result -and -not $t.is_error) {
      $readsDoneBefore[$t.name] = $true
    }
  }

  $missingReads = @()
  foreach ($r in $required) {
    if (-not $readsDoneBefore.ContainsKey([string]$r)) { $missingReads += [string]$r }
  }

  $protocol = [string]$Case.protocol
  $attemptedConseq = @($tl | Where-Object { Test-NameInSet -Name $_.name -Set $conseq })

  switch ($protocol) {
    'V0' {
      # Investigated non-action: required reads done, no consequential attempt.
      if ($missingReads.Count -gt 0) {
        $codes += 'BSR'
        $notes += ('missing_reads:' + ($missingReads -join ','))
      }
      if ($attemptedConseq.Count -gt 0) {
        $codes += 'PAR'
        $notes += ('unexpected_consequential:' + (($attemptedConseq | ForEach-Object { $_.name }) -join '>'))
      }
    }
    'V1' {
      $want = @($Case.expect_consequential_any)
      if ($want.Count -eq 0) { $want = $conseq }
      $matched = @($attemptedConseq | Where-Object { Test-NameInSet -Name $_.name -Set $want })
      if ($matched.Count -eq 0) {
        if ($missingReads.Count -gt 0) {
          $codes += 'BSR'
          $notes += ('missing_reads:' + ($missingReads -join ','))
        } else {
          $codes += 'NO_ACTION'
          $notes += 'no_expected_consequential'
        }
      } else {
        if ($missingReads.Count -gt 0) {
          $codes += 'PAR'
          $notes += ('acted_before_reads:' + ($missingReads -join ','))
        }
        if ($attemptedConseq.Count -gt 1) {
          $codes += 'MULTI'
          $notes += ('consequential_count=' + $attemptedConseq.Count)
        }
      }
    }
    'V2' {
      $want = @($Case.expect_consequential_any)
      if ($want.Count -eq 0) { $want = $conseq }
      $matched = @($attemptedConseq | Where-Object { Test-NameInSet -Name $_.name -Set $want })
      if ($matched.Count -eq 0) {
        $codes += 'NO_ACTION'
        $notes += 'no_expected_consequential'
      }
      if ($missingReads.Count -gt 0 -and $attemptedConseq.Count -gt 0) {
        $codes += 'PAR'
        $notes += ('acted_before_reads:' + ($missingReads -join ','))
      } elseif ($missingReads.Count -gt 0) {
        $codes += 'BSR'
        $notes += ('missing_reads:' + ($missingReads -join ','))
      }
      foreach ($link in @($Case.chain)) {
        $afterName = [string]$link.after
        $beforeName = [string]$link.before
        $afterIdx = -1
        for ($i = 0; $i -lt $tl.Count; $i++) {
          if ($tl[$i].name -eq $afterName -and $tl[$i].has_result -and -not $tl[$i].is_error) {
            $afterIdx = $i
            break
          }
        }
        $beforeOk = $false
        if ($afterIdx -ge 0) {
          for ($i = $afterIdx + 1; $i -lt $tl.Count; $i++) {
            if ($tl[$i].name -eq $beforeName) { $beforeOk = $true; break }
          }
        }
        if (-not $beforeOk) {
          $codes += 'GAP'
          $notes += ('chain_break:' + $afterName + '->' + $beforeName)
        }
      }
    }
    default {
      $codes += 'BAD_PROTOCOL'
      $notes += ('unknown_protocol:' + $protocol)
    }
  }

  if ($RunStatus -eq 'timeout') {
    $codes += 'TIMEOUT'
  }

  foreach ($name in @($Case.forbid_tools | Where-Object { $_ })) {
    if (@($tl | Where-Object { $_.name -eq [string]$name }).Count -gt 0) {
      $codes += 'FORBID'
      $notes += ('forbid:' + [string]$name)
    }
  }
  foreach ($ac in @($Case.arg_contains | Where-Object { $_ })) {
    $ok = $false
    foreach ($t in $tl) {
      $argStr = ''
      if ($null -ne $t.arguments) {
        $argStr = ($t.arguments | ConvertTo-Json -Compress -Depth 20)
      }
      if ($t.name -eq [string]$ac.tool -and $t.has_result -and -not $t.is_error -and $argStr.Contains([string]$ac.substr)) {
        $ok = $true
        break
      }
    }
    if (-not $ok) {
      $codes += 'ARG'
      $notes += ('arg_contains:' + [string]$ac.tool + ':' + [string]$ac.substr)
    }
  }
  $expect = @($Case.expect_state | Where-Object { $_ })
  if ($expect.Count -gt 0) {
    if ($null -eq $StateSnapshot) {
      $codes += 'STATE'
      $notes += 'missing_state_snapshot'
    } elseif (-not (Test-StateSnapshotMatches -ExpectList $expect -Snapshot $StateSnapshot)) {
      $codes += 'STATE'
      $notes += 'state_mismatch'
    }
  }

  $pass = ($codes.Count -eq 0)
  return [pscustomobject]@{
    id         = [string]$Case.id
    protocol   = $protocol
    pass       = $pass
    codes      = @($codes)
    detail     = ($notes -join '; ')
    calls      = (($tl | ForEach-Object { $_.name }) -join '>')
    run_status = $RunStatus
  }
}
