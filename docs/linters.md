# linters

### Why linters is returning `return 1` after each command
There is currently a bash issue where the exit code is not getting detected when there is multiple commands being ran. 
The easy solution for now is to return the value 1 from the function indicating an error has occured and terminating the `make test/lint` command that triggered it
