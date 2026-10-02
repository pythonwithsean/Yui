# Guidance for Coding Agents

## Learning-first networking changes

This repository is a learning project for understanding HTTP/1.1 over raw TCP.
When changing request reading or parsing, explain the idea and the tradeoffs
before implementing a substantial behavior change. Do not replace the current
byte-by-byte reader with a buffered reader, or change timeout and size-limit
behavior, without first showing how the proposed design works and getting
confirmation.

The request reader must preserve these safety properties:

- Enforce a maximum header size while the header is being accumulated.
- Use a default timeout so a client cannot send a few bytes and hold a
  connection forever.
- Read exactly the declared body length when `Content-Length` is used.
- Preserve bytes already read past the end of the headers for body parsing.

When discussing a possible improvement, cover:

1. Why a larger read buffer reduces system calls without changing TCP message
   boundaries.
2. How `bufio.Reader` buffers bytes and lets parsing read ahead safely with
   `Peek`, `ReadSlice`, or `ReadBytes`.
3. The difference between a total connection deadline and an idle/read
   deadline.
4. Where the maximum header size is checked and how an oversized request is
   rejected.

Do not treat this file as permission to make those changes automatically. The
goal is to teach the design first, then implement one measured step at a time.
