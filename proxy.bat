@echo off

set PROXY_SERVER=socks5://127.0.0.1:10808
set HTTP_PROXY=http://127.0.0.1:10809
set HTTPS_PROXY=http://127.0.0.1:10809

rem Set environment variables
setx ALL_PROXY %PROXY_SERVER%
setx all_proxy %PROXY_SERVER%

rem Show current IP address (without proxy)
echo Current IP address (without proxy):
curl myip.ipip.net

rem Set Git proxy
git config --global http.proxy %PROXY_SERVER%
git config --global https.proxy %PROXY_SERVER%

git config --global http.proxy socks5 %PROXY_SERVER%
git config --global https.proxy socks5 %PROXY_SERVER%

rem Set temporary environment variables to check proxy
set ALL_PROXY=%PROXY_SERVER%
set all_proxy=%PROXY_SERVER%

rem Check if proxy is working
echo IP address (with proxy):
curl myip.ipip.net

echo Proxy has been set to %PROXY_SERVER%