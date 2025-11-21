# DuoHelper
A tool to alert me that I need to complete my daily language learning lesson on Duolingo, if it is not done by a certain time.

The project is a WIP and will only work on my PC as it is right now. 

## Functionality
Currently, it needs the jwt_token to be provided inside of `tokens.json`. Using this, it can authenticate an API request to the undocumented Duolingo API to see if the streak has been extended today. 

## TO DO
- Automate checking with Windows Scheduler
- Send an answer via a Windows notification
- When the JWT token expires, send a prompt to log in via the Windows notification. Extract the token after login - this will keep manual effort low. 
