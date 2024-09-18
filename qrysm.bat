@echo off

SetLocal EnableDelayedExpansion & REM All variables are set local to this run & expanded at execution time rather than at parse time (tip: echo !output!)

set THEQRL_SIGNING_KEY=0AE0051D647BA3C1A917AF4072E33E4DF1A5036E

REM Complain if invalid arguments were provided.
for %%a in (beacon-chain validator client-stats) do (
    if %1 equ %%a (
        goto validprocess
    )
)
echo [31mERROR: PROCESS missing or invalid[0m
echo Usage: ./qrysm.bat PROCESS FLAGS.
echo.
echo PROCESS can be beacon-chain, validator, or client-stats.
echo FLAGS are the flags or arguments passed to the PROCESS.
echo. 
echo Use this script to download the latest Qrysm release binaries.
echo Downloaded binaries are saved to .\dist
echo. 
echo To specify a specific release version:
echo  set USE_QRYSM_VERSION=v1.0.0-alpha3
echo  to resume using the latest release:
echo   set USE_QRYSM_VERSION=
echo.
echo To automatically restart crashed processes:
echo  set QRYSM_AUTORESTART=true^& .\qrysm.bat beacon-chain
echo  to stop autorestart run:
echo   set QRYSM_AUTORESTART=
echo. 
exit /B 1
:validprocess

REM Get full path to qrysm.bat file (excluding filename)
set wrapper_dir=%~dp1dist
reg Query "HKLM\Hardware\Description\System\CentralProcessor\0" | find /i "x86" > NUL && set WinOS=32BIT || set WinOS=64BIT
if %WinOS%==32BIT (
    echo [31mERROR: qrysm is only supported on 64-bit Operating Systems [0m
    exit /b 1
)
if %WinOS%==64BIT (
    set arch=amd64.exe
    set system=windows
)

mkdir %wrapper_dir%

REM get_qrysm_version - Find the latest Qrysm version available for download.
:: TODO(now.youtrack.cloud/issue/TQ-1)
(for /f %%i in ('curl -f -s https://prysmaticlabs.com/releases/latest') do set qrysm_version=%%i) || (echo [31mERROR: Starting qrysm requires an internet connection. If you are being blocked by your antivirus, you can download the beacon chain and validator executables from our releases page on Github here https://github.com/theQRL/qrysm/releases/ [0m && exit /b 1)
set qrysm_version="v0.1.1"
echo [37mLatest qrysm release is %qrysm_version%.[0m
IF defined USE_QRYSM_VERSION (
    echo [33mdetected variable USE_QRYSM_VERSION=%USE_QRYSM_VERSION%[0m
    set reason=as specified in USE_QRYSM_VERSION
    set qrysm_version=%USE_QRYSM_VERSION%
) else (
    set reason=automatically selected latest available release
)
echo Using qrysm version %qrysm_version%.

set BEACON_CHAIN_REAL=%wrapper_dir%\beacon-chain-%qrysm_version%-%system%-%arch%
set VALIDATOR_REAL=%wrapper_dir%\validator-%qrysm_version%-%system%-%arch%
set CLIENT_STATS_REAL=%wrapper_dir%\client-stats-%qrysm_version%-%system%-%arch%

if "%~1"=="beacon-chain" (
    if exist "%BEACON_CHAIN_REAL%" (
        echo [32mBeacon chain is up to date.[0m
    ) else (
        echo [35mDownloading beacon chain %qrysm_version% to %BEACON_CHAIN_REAL% %reason%[0m
        for /f "delims=" %%i in ('curl --silent -o nul -w "%%{http_code}" https://github.com/theQRL/qrysm/releases/download/%qrysm_version%/beacon-chain-%qrysm_version%-%system%-%arch% ') do set "http=%%i" && echo %%i
		if "!http!"=="404" (
			echo [35mNo qrysm beacon chain found for %qrysm_version%[0m
			exit /b 1
		)	
		curl -L https://github.com/theQRL/qrysm/releases/download/%qrysm_version%/beacon-chain-%qrysm_version%-%system%-%arch% -o %BEACON_CHAIN_REAL%
		curl --silent -L https://github.com/theQRL/qrysm/releases/download/%qrysm_version%/beacon-chain-%qrysm_version%-%system%-%arch%.sha256 -o %wrapper_dir%\beacon-chain-%qrysm_version%-%system%-%arch%.sha256
		curl --silent -L https://github.com/theQRL/qrysm/releases/download/%qrysm_version%/beacon-chain-%qrysm_version%-%system%-%arch%.sig -o %wrapper_dir%\beacon-chain-%qrysm_version%-%system%-%arch%.sig
    )
)

if "%~1"=="validator" (
    if exist "%VALIDATOR_REAL%" (
        echo [32mValidator is up to date.[0m
    ) else (
        echo [35mDownloading validator %qrysm_version% to %VALIDATOR_REAL% %reason%[0m
		for /f "delims=" %%i in ('curl --silent -o nul -w "%%{http_code}" https://github.com/theQRL/qrysm/releases/download/%qrysm_version%/validator-%qrysm_version%-%system%-%arch% ') do set "http=%%i" && echo %%i
		if "!http!"=="404"  (
			echo [35mNo qrysm validator found for %qrysm_version%[0m
			exit /b 1
		)
		curl -L https://github.com/theQRL/qrysm/releases/download/%qrysm_version%/validator-%qrysm_version%-%system%-%arch% -o %VALIDATOR_REAL%
        curl --silent -L https://github.com/theQRL/qrysm/releases/download/%qrysm_version%/validator-%qrysm_version%-%system%-%arch%.sha256 -o %wrapper_dir%\validator-%qrysm_version%-%system%-%arch%.sha256
        curl --silent -L https://github.com/theQRL/qrysm/releases/download/%qrysm_version%/validator-%qrysm_version%-%system%-%arch%.sig -o %wrapper_dir%\validator-%qrysm_version%-%system%-%arch%.sig
    )
)

if "%~1"=="client-stats" (
    if exist %CLIENT_STATS_REAL% (
        echo [32mClient-stats is up to date.[0m
    ) else (
        echo [35mDownloading client-stats %qrysm_version% to %CLIENT_STATS_REAL% %reason%[0m
		for /f "delims=" %%i in ('curl --silent -o nul -w "%%{http_code}" https://github.com/theQRL/qrysm/releases/download/%qrysm_version%/client-stats-%qrysm_version%-%system%-%arch% ') do set "http=%%i" && echo %%i
		if "!http!"=="404" (
			echo [35mNo qrysm client stats found for %qrysm_version%[0m
			exit /b 1
		)
		curl -L https://github.com/theQRL/qrysm/releases/download/%qrysm_version%/client-stats-%qrysm_version%-%system%-%arch% -o %CLIENT_STATS_REAL%
        curl --silent -L https://github.com/theQRL/qrysm/releases/download/%qrysm_version%/client-stats-%qrysm_version%-%system%-%arch%.sha256 -o %wrapper_dir%\client-stats-%qrysm_version%-%system%-%arch%.sha256
        curl --silent -L https://github.com/theQRL/qrysm/releases/download/%qrysm_version%/client-stats-%qrysm_version%-%system%-%arch%.sig -o %wrapper_dir%\client-stats-%qrysm_version%-%system%-%arch%.sig
    )
)

if "%~1"=="slasher" (
    # TODO(now.youtrack.cloud/issue/TQ-1)
    echo [31mThe slasher binary is no longer available. Please use the --slasher flag with your beacon node. See: https://docs.prylabs.network/docs/prysm-usage/slasher/[0m
    exit /b 1
)

if "%~1"=="beacon-chain" ( set process=%BEACON_CHAIN_REAL%)
if "%~1"=="validator" ( set process=%VALIDATOR_REAL%) 
if "%~1"=="client-stats" ( set process=%CLIENT_STATS_REAL%)

REM GPG not natively available on Windows, external module required
echo [33mWARN GPG verification is not natively available on Windows.[0m
echo [33mWARN Skipping integrity verification of downloaded binary[0m
REM Check SHA256 File Hash before running
echo [37mVerifying binary authenticity with SHA256 Hash.[0m
for /f "delims=" %%A in ('certutil -hashfile %process% SHA256 ^| find /v "hash"') do (
    set SHA256Hash=%%A
)
set /p ExpectedSHA256=<%process%.sha256
if "%ExpectedSHA256:~0,64%"=="%SHA256Hash%" (
    echo [32mSHA256 Hash Match![0m
) else if "%QRYSM_ALLOW_UNVERIFIED_BINARIES%"=="1" (
    echo [31mWARNING Failed to verify Qrysm binary.[0m 
    echo Detected QRYSM_ALLOW_UNVERIFIED_BINARIES=1
    echo Proceeding...
) else (
    echo [31mERROR Failed to verify Qrysm binary. Please erase downloads in the
    echo dist directory and run this script again. Alternatively, you can use a
    echo A prior version by specifying environment variable USE_QRYSM_VERSION
    echo with the specific version, as desired. Example: set USE_QRYSM_VERSION=v1.0.0-alpha.5
    echo If you must wish to continue running an unverified binary, use:
    echo set QRYSM_ALLOW_UNVERIFIED_BINARIES=1[0m
    exit /b 1
)

set processargs=%*
echo Starting Qrysm %processargs%

set "processargs=!processargs:*%1=!" & REM remove process from the list of arguments

:autorestart
    %process% %processargs% 
    if ERRORLEVEL 1 goto :ERROR
    REM process terminated gracefully
    pause
    exit /b 0

:ERROR
    Echo [91mERROR FAILED[0m
    IF defined QRYSM_AUTORESTART (
        echo QRYSM_autorestart is set, restarting
        GOTO autorestart
    ) else (
        echo an error has occured, set QRYSM_AUTORESTART=1 to automatically restart
    )

:end
REM Variables are set local to this run
EndLocal
