$ErrorActionPreference = 'Stop'

$packageName = 'iso-auto-downloader'
$toolsDir    = "$(Split-Path -parent $MyInvocation.MyCommand.Definition)"
$version     = '0.1.0'

$packageArgs = @{
  packageName    = $packageName
  unzipLocation  = $toolsDir
  url64bit       = "https://github.com/arnaudcharles/iso-auto-downloader/releases/download/v$version/iso-auto-downloader_${version}_windows_amd64.zip"
  checksum64     = '2fcd8611dc4252534c6f4b5f2bcef261391e2cbec8fc420fad40af8f7a89a19a'
  checksumType64 = 'sha256'
}

Install-ChocolateyZipPackage @packageArgs
