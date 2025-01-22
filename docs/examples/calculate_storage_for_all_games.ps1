# To run the script, open PowerShell and execute the following command:
# Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass; .\calculate_storage_all_games.ps1

# Install PowerShell on Linux
# sudo apt-get install -y powershell # Debian-based distros
# sudo yum install -y powershell # Fedora
# sudo zypper install -y powershell # openSUSE
# sudo pacman -S powershell # Arch Linux
# sudo snap install powershell --classic # Ubuntu

# Colors
$RED = "`e[31m"
$GREEN = "`e[32m"
$YELLOW = "`e[33m"
$NC = "`e[0m" # No Color

Write-Host "${GREEN}========================== Download All Games (Powershell Script) ===================================${NC}"
Write-Host "${GREEN}Calculate the storage size for downloading all games owned by the user on GOG.com with given options.${NC}"
Write-Host "${GREEN}=====================================================================================================${NC}"

$env:DEBUG_GOGG = 0 # Debug mode disabled
$GOGG = ".\bin/gogg" # Path to Gogg's executable file (for example, ".\bin\gogg")

# Download options
$LANG = "en" # Language English
$PLATFORM = "windows" # Platform Windows
$INCLUDE_DLC = 1 # Include DLCs
$INCLUDE_EXTRA_CONTENT = 1 # Include extra content
$STORAGE_UNIT = "GB" # Storage unit (MB or GB)

# Function to clean up the CSV file
function Cleanup
{
    if ($latest_csv)
    {
        Remove-Item -Force $latest_csv
        if ($?)
        {
            Write-Host "${RED}Cleanup: removed $latest_csv${NC}"
        }
    }
}

# Update game catalogue and export it to a CSV file
& $GOGG catalogue refresh
& $GOGG catalogue export ./ --format=csv

# Find the newest catalogue file
$latest_csv = Get-ChildItem -Path . -Filter "gogg_catalogue_*.csv" | Sort-Object LastWriteTime -Descending | Select-Object -First 1

# Check if the catalogue file exists
if (-not $latest_csv)
{
    Write-Host "${RED}No CSV file found.${NC}"
    exit 1
}

Write-Host "${GREEN}Using catalogue file: $( $latest_csv.Name )${NC}"

# Initialize counter and total size
$counter = 0
$totalSize = 0.0

# Download each game listed in catalogue file, skipping the first line
Get-Content $latest_csv.FullName | Select-Object -Skip 1 | ForEach-Object {
    $fields = $_ -split ","
    $game_id = $fields[0]
    $game_title = $fields[1]
    $counter++
    Write-Host "${YELLOW}${counter}: Game ID: $game_id, Title: $game_title${NC}"
    $sizeOutput = & $GOGG file size $game_id --platform=$PLATFORM --lang=$LANG --dlcs=$INCLUDE_DLC `
    --extras=$INCLUDE_EXTRA_CONTENT --unit=$STORAGE_UNIT
    $size = [double]($sizeOutput -replace '[^\d.]', '')
    $totalSize += $size
    Write-Host "${YELLOW}Total download size: $size $STORAGE_UNIT${NC}"
    Start-Sleep -Seconds 0.0
}

# Print total download size
Write-Host "${GREEN}Total download size for all games: $totalSize $STORAGE_UNIT${NC}"

# Clean up
Cleanup
