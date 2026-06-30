# new-api Context

This context defines project-specific language for the API gateway and the Canvas-facing generation workflows.

## Language

**Canvas Jimeng Generation Entry**:
A Canvas-facing generation capability that exposes Jimeng-style image or video generation through the gateway's multi-user API surface. It uses the Jimeng CLI guide as an experience and capability reference, but does not mean the backend directly executes a local `dreamina` CLI process.
_Avoid_: Jimeng CLI backend, local dreamina executor

**Jimeng Metadata Schema**:
A typed extension namespace inside Canvas relay requests for Jimeng-specific generation options that do not fit the shared OpenAI-compatible fields. It is a deliberate request contract, not an unstructured escape hatch.
_Avoid_: Free-form metadata blob, Jimeng custom client

**Jimeng Account Authorization**:
A user-linked authorization relationship that allows the gateway to act on behalf of a specific user's Jimeng web account. It is an account binding flow between the user and the system, not a server-local CLI login session.
_Avoid_: dreamina login, server CLI session

**Dreamina OAuth HTTP Adaptor**:
A provider adaptor that uses a Jimeng account authorization to call Dreamina's remote HTTP generation, upload, and query protocols from the gateway. It is the preferred backend integration shape when those protocols are verified, distinct from spawning the local `dreamina` CLI as an executor.
_Avoid_: CLI wrapper, shell command adaptor
