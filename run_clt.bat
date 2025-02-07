@echo off
cd client
if "%1" == "" (
    go run main.go http://rps-e20j.onrender.com GOClient1
) else (
   go run main.go %1 GOClient1 
)
