# goscp-server

### A Go implementation of the kSoft kSCP server protocol

----

## What's working?
- Public and private messaging
- Pong responses to client pings
- Join authentication
- Single-client usernames
- Announcements for joining, leaving, name changes, etc
- AFK support
- User lists
- User flags
- User agents
- Username changing
- Graceful disconnects
- Message of the Day

## What's left?
- RAW opcode implementation (allows for file transfers, client-side plugin communication with other users, etc)
- Admin and root authentication support
- Kicking/banning users by username or IP
- Pinging users at a set interval
- Setting Message of the Day from client
- Muting users
- Shutting down or restarting server by command with timeout
- Rehash server configuration

----

## Support
For help and support with GoSCP Server, file a previously unfiled issue on the issues page.

## License
The source code for GoSCP Server is released under the MIT License. See LICENSE for more details.

## Documentation
The documentation and specification of the Simple Chat Protocol revision 9 used for GoSCP Server can be found in [Simple Chat Protocol Documentation and Specification.txt](Simple%20Chat%20Protocol%20Documentation%20and%20Specification.txt).

## Donations
All donations are highly appreciated. They help me pay for the server costs to keep development builds of GoSCP Server running and even help point my attention span to GoSCP Server to fix issues and implement newer and better features!

[![Donate](https://img.shields.io/badge/Donate-PayPal-green.svg)](https://paypal.me/JoshuaDoes)
