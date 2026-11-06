$ErrorActionPreference = 'Stop'
$utf8 = [System.Text.UTF8Encoding]::new($false)
$temp = Join-Path $env:LOCALAPPDATA "Temp\opencode\ctrld-version-$([guid]::NewGuid())"
$null = New-Item -ItemType Directory -Path $temp
$scriptPath = Join-Path $temp 'build.ps1'
$versionPath = Join-Path $temp 'VERSION'
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'build.ps1') -Destination $scriptPath

# 模擬編譯器，測試版本、輸出位置與失敗處理；實際編譯另由 build.ps1 驗證。
$buildArguments = [System.Collections.Generic.List[string]]::new()
function go {
    foreach ($argument in $args) {
        $buildArguments.Add([string]$argument)
    }
    $global:LASTEXITCODE = $compilerExit
}

$zone = [System.TimeZoneInfo]::FindSystemTimeZoneById('Taipei Standard Time')
$today = [System.TimeZoneInfo]::ConvertTime([datetimeoffset]::UtcNow, $zone)
$prefix = "v$($today.Year).$($today.DayOfYear)"
$tests = @(
    @{ Name = 'rebuild preserves version'; Before = 'v2024.366.7'; After = 'v2024.366.7'; Bump = $false },
    @{ Name = 'same day increments'; Before = "$prefix.1"; After = "$prefix.2"; Bump = $true },
    @{ Name = 'different year resets'; Before = 'v2000.1.8'; After = "$prefix.1"; Bump = $true },
    @{ Name = 'day starts at one'; Before = 'v2026.0.1'; Fails = $true },
    @{ Name = 'revision starts at one'; Before = 'v2026.1.0'; Fails = $true },
    @{ Name = 'no day padding'; Before = 'v2026.001.1'; Fails = $true },
    @{ Name = 'nonleap day invalid'; Before = 'v2025.366.1'; Fails = $true },
    @{ Name = 'leap day limit'; Before = 'v2024.367.1'; Fails = $true },
    @{ Name = 'year must be valid'; Before = 'v0000.1.1'; Fails = $true },
    @{ Name = 'format invalid'; Before = 'invalid'; Fails = $true },
    @{ Name = 'build failure preserves bumped version'; Before = "$prefix.1"; After = "$prefix.2"; Bump = $true; Fails = $true; Exit = 1 }
)
$yesterday = $today.AddDays(-1)
$tests += @{ Name = 'different day resets'; Before = "v$($yesterday.Year).$($yesterday.DayOfYear).9"; After = "$prefix.1"; Bump = $true }
$tests += @{ Name = 'default output creates dist'; Before = "$prefix.1" }
$tests += @{ Name = 'custom output is preserved'; Before = "$prefix.1"; Output = 'custom.exe' }

try {
    foreach ($test in $tests) {
        $distPath = Join-Path $temp 'dist'
        if (Test-Path -LiteralPath $distPath) {
            Remove-Item -LiteralPath $distPath
        }
        $buildArguments.Clear()
        [System.IO.File]::WriteAllText($versionPath, "$($test.Before)`r`n", $utf8)
        $compilerExit = if ($test.Exit) { $test.Exit } else { 0 }
        $failed = $false
        try {
            $parameters = @{ BumpVersion = [bool]$test.Bump }
            if ($test.ContainsKey('Output')) {
                $parameters.Output = $test.Output
            }
            & $scriptPath @parameters | Out-Null
        }
        catch {
            $failed = $true
        }
        if ($failed -ne [bool]$test.Fails) {
            throw "Unexpected result: $($test.Name)"
        }
        if (-not $failed) {
            $expectedOutput = if ($test.ContainsKey('Output')) { $test.Output } else { 'dist/ctrld-openapi-doc.exe' }
            if (($buildArguments -join '|') -ne "build|-trimpath|-o|$expectedOutput|.") {
                throw "Build arguments mismatch: $($test.Name): $buildArguments"
            }
            $expectDist = -not $test.ContainsKey('Output')
            if ((Test-Path -LiteralPath $distPath -PathType Container) -ne $expectDist) {
                throw "Output directory mismatch: $($test.Name)"
            }
        }
        $expected = if ($test.After) { $test.After } else { $test.Before }
        $actual = [System.IO.File]::ReadAllText($versionPath).Trim()
        if ($actual -ne $expected) {
            throw "Version mismatch: $($test.Name): $actual != $expected"
        }
        $bytes = [System.IO.File]::ReadAllBytes($versionPath)
        if ($bytes[0] -eq 0xEF -or $bytes[-2] -ne 13 -or $bytes[-1] -ne 10) {
            throw "Encoding or newline mismatch: $($test.Name)"
        }
        Write-Output "PASS $($test.Name)"
    }
    Write-Output "Build tests: $($tests.Count) passed"
}
finally {
    Remove-Item -LiteralPath $temp -Recurse -Force
    $global:LASTEXITCODE = 0
}
