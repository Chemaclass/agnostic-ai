### General

- Typing into a chat through the local server, and running a terminal command to resume a chat from it, need the companion's secret or the key of a tab `view` opened. Sending from scripts or other headless callers is retired. A server with neither, such as a project server, refuses every send, and an open without proof returns the resume command to run by hand (#3930).
