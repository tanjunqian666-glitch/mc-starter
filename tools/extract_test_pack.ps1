# PowerShell script to extract test pack
# Note: 仔の家 has CJK chars that get mangled via SSH, so we use codepage trick
$desktop = [Environment]::GetFolderPath('Desktop')

# Find the zip on desktop
$zips = @(Get-ChildItem -Path $desktop -Filter "*.zip" | Where-Object { $_.Name -like "*小猪*" })
if ($zips.Count -eq 0) {
    Write-Host "ERROR: No matching zip found on desktop!"
    exit 1
}
$zip = $zips[0].FullName
$baseName = $zip -replace '\.zip$', ''
Write-Host "Found: $zip"
Write-Host "Dest: $baseName"

Expand-Archive -Path $zip -DestinationPath $baseName -Force
Write-Host "Extract done. Contents:"
Get-ChildItem $baseName | Select-Object Name, Length, PSIsContainer | Format-Table -AutoSize
