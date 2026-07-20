$ErrorActionPreference = 'Stop'

$packageName = 'iso-auto-downloader'
$toolsDir    = "$(Split-Path -parent $MyInvocation.MyCommand.Definition)"
$version     = '0.1.1'

$packageArgs = @{
  packageName    = $packageName
  unzipLocation  = $toolsDir
  url64bit       = "https://github.com/arnaudcharles/iso-auto-downloader/releases/download/v$version/iso-auto-downloader_${version}_windows_amd64.zip"
  checksum64     = '5f9e4c6f87d462452665e9eeefb8a3b9cc6ff2ff00213ad976fcbe0a11ee6813'
  checksumType64 = 'sha256'
}

Install-ChocolateyZipPackage @packageArgs
