#!/bin/bash
clear
echo "Type in username:"
read user
echo "Type in the last byte of the IP to the elvator you want to connect to:"
read IP
echo "Connecting to 129.241.187."$IP
scp -rq Project/. $user@129.241.187.$IP:~/TTK4145
ssh $user@129.241.187.$IP
#TODO: Needs to be run inside ssh:
	#cd TTK4145/
	#go run main.go