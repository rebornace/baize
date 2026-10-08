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
      for ($j = $timeline.Count - 1; $j -ge 0; $j--) {
        $t = $timeline[$j]
        if ($t.name -eq $name -and -not $t.has_result) {
          $t.hitl_waiting = $true
          break
        }
      }
    } elseif ($ev.type -eq 'tool.result') {
      $id = [string]$ev.data.id
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
function Score-EvidenceCase {
  param(
    $Case,
    $Timeline,
    [string]$RunStatus = ''
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
