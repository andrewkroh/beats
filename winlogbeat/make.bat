@echo off

REM Windows wrapper for Mage (https://magefile.org/) that installs the version
REM defined in go.mod to %GOPATH%\bin.
REM
REM After running this once you may invoke mage.exe directly.

WHERE mage
IF %ERRORLEVEL% NEQ 0 go install github.com/magefile/mage

mage %*
