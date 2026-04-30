# Find zips on desktop
$d = [Environment]::GetFolderPath('Desktop')
Get-ChildItem $d -Filter *.zip | ForEach-Object { $_.FullName + "|" + $_.Name }
