$ErrorActionPreference = 'Stop'

$packageName = 'iso-auto-downloader'
$toolsDir    = "$(Split-Path -parent $MyInvocation.MyCommand.Definition)"

Remove-Item -Recurse -Force "$toolsDir\iso-auto-downloader.exe" -ErrorAction SilentlyContinue
