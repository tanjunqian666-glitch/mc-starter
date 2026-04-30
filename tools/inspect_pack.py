import paramiko
import os
import base64
import time

def run_powershell(ssh, ps_code):
    """Run PowerShell via EncodedCommand to avoid all quoting issues."""
    encoded = base64.b64encode(ps_code.encode('utf-16-le')).decode()
    stdin, stdout, stderr = ssh.exec_command(f'powershell -EncodedCommand {encoded}')
    out = stdout.read().decode('utf-8', errors='replace')
    err = stderr.read().decode('utf-8', errors='replace')
    return out, err

def main():
    ssh = paramiko.SSHClient()
    ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    ssh.connect('192.168.139.132', 22, 'claw', '12345678')

    # Step 1: find the zip on desktop
    find_script = """
$d = [Environment]::GetFolderPath('Desktop')
Get-ChildItem $d -Filter *.zip | ForEach-Object {
    Write-Output ('NAME:' + $_.Name)
    Write-Output ('FULL:' + $_.FullName)
}
"""
    out, _ = run_powershell(ssh, find_script)
    print("=== Find ZIP output ===")
    print(out)

    # Parse the output
    lines = [l.strip() for l in out.split('\n') if l.strip()]
    zip_name = None
    zip_full = None
    for i, l in enumerate(lines):
        if l.startswith('NAME:'):
            zip_name = l[5:]
        if l.startswith('FULL:'):
            zip_full = l[5:]
    print(f"\nFound: name={zip_name}")
    print(f"Full: {zip_full}")

    if not zip_full:
        print("ERROR: No zip found!")
        ssh.close()
        return

    # Step 2: extract using Expand-Archive with the full path
    extract_script = f"""
$zip = '{zip_full}'
$dest = $zip -replace '\\.zip$', ''
Write-Output "Extracting: $zip -> $dest"
Expand-Archive -Path $zip -DestinationPath $dest -Force
Write-Output "Done."
"""
    out, err = run_powershell(ssh, extract_script)
    print("\n=== Extract output ===")
    print(out)
    if err.strip() and '<CLIXML' not in err:
        print("ERR:", err)

    # Step 3: list contents
    list_script = f"""
$dest = '{zip_full}' -replace '\\.zip$', ''
Get-ChildItem $dest | ForEach-Object {{
    Write-Output ('  ' + $_.Name + ' | ' + ('dir' if $_.PSIsContainer else '{0:N0}'.f($_.Length)) + ' | ' + $_.LastWriteTime.ToString('yyyy-MM-dd HH:mm'))
}}
"""
    out, _ = run_powershell(ssh, list_script)
    print("\n=== Contents ===")
    print(out)

    # Step 4: get full tree
    tree_script = f"""
$dest = '{zip_full}' -replace '\\.zip$', ''
Get-ChildItem $dest -Recurse -Depth 2 | ForEach-Object {{
    Write-Output ('  ' + '  ' * ($_.DirectoryName.Length - $dest.Length) + $_.Name)
}}
"""
    out, _ = run_powershell(ssh, tree_script)
    print("\n=== Tree (depth 2) ===")
    print(out)

    # Step 5: check for .minecraft structure
    check_script = f"""
$dest = '{zip_full}' -replace '\\.zip$', ''
Get-ChildItem $dest -Recurse -Depth 0 | ForEach-Object {{
    Write-Output $_.FullName
}}
"""
    out, _ = run_powershell(ssh, check_script)
    print("\n=== Full paths ===")
    print(out)

    ssh.close()

if __name__ == '__main__':
    main()
