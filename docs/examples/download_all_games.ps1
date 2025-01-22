# To run the script, open PowerShell and execute the following command:
# Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass; .\download_all_games.ps1

# Colors
$RED = "`e[31m"
$GREEN = "`e[32m"
$YELLOW = "`e[33m"
$NC = "`e[0m" # No Color

Write-Host "${GREEN}===================== Download All Games (Windows Powershell Version) ======================${NC}"
Write-Host "${GREEN}The code in this script downloads all games owned by the user on GOG.com with given options.${NC}"
Write-Host "${GREEN}============================================================================================${NC}"

$env:DEBUG_GOGG = 1 # Debug mode enabled
$GOGG = ".\bin/gogg" # Path to Gogg's executable file (for example, ".\bin\gogg")

# Download options
$LANG = "en" # Language English
$PLATFORM = "windows" # Platform Windows
$INCLUDE_DLC = 1 # Include DLCs
$INCLUDE_EXTRA_CONTENT = 1 # Include extra content
$RESUME_DOWNLOAD = 1 # Resume download
$NUM_THREADS = 4 # Number of worker threads for downloading
$FLATTEN = 1 # Flatten directory structure
$OUTPUT_DIR = "./games" # Output directory

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

# Download each game listed in catalogue file, skipping the first line
Get-Content $latest_csv.FullName | Select-Object -Skip 1 | ForEach-Object {
    $fields = $_ -split ","
    $game_id = $fields[0]
    $game_title = $fields[1]
    Write-Host "${YELLOW}Game ID: $game_id, Title: $game_title${NC}"
    & $GOGG download $game_id $OUTPUT_DIR --platform=$PLATFORM --lang=$LANG `
        --dlcs=$INCLUDE_DLC --extras=$INCLUDE_EXTRA_CONTENT --resume=$RESUME_DOWNLOAD --threads=$NUM_THREADS `
        --flatten=$FLATTEN
    Start-Sleep -Seconds 1
}

# Clean up
Cleanup
