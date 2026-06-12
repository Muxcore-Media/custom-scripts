#!/usr/bin/env python3
"""MCP stdio server for muxidx vector search."""

import json
import os
import sys
import traceback

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from embedder import Embedder
from store import Store


def send_response(req_id, result):
    if req_id is not None:
        msg = json.dumps({"jsonrpc": "2.0", "id": req_id, "result": result})
        sys.stdout.write(msg + "\n")
        sys.stdout.flush()


def send_error(req_id, code, message):
    if req_id is not None:
        msg = json.dumps({"jsonrpc": "2.0", "id": req_id, "error": {"code": code, "message": message}})
        sys.stdout.write(msg + "\n")
        sys.stdout.flush()


def handle_request(req, store, embedder):
    req_id = req.get("id")
    method = req.get("method", "")
    params = req.get("params", {})

    if method == "initialize":
        protocol_version = params.get("protocolVersion", "2024-11-05")
        send_response(req_id, {
            "protocolVersion": protocol_version,
            "capabilities": {
                "tools": {},
            },
            "serverInfo": {
                "name": "muxidx",
                "version": "1.0.0",
            },
        })

    elif method == "notifications/initialized":
        pass

    elif method in ("tools/list", "mcp.tools.list"):
        send_response(req_id, {
            "tools": [
                {
                    "name": "search",
                    "description": "Semantic search across MuxCore codebase and wiki",
                    "inputSchema": {
                        "type": "object",
                        "properties": {
                            "query": {"type": "string", "description": "Natural language query"},
                            "repo": {"type": "string", "description": "Filter: core, wiki, or all", "default": "all"},
                            "top_k": {"type": "integer", "description": "Number of results", "default": 10},
                            "include_graph": {"type": "boolean", "description": "Include graph neighbors", "default": True},
                        },
                        "required": ["query"],
                    }
                },
                {
                    "name": "graph_walk",
                    "description": "Walk knowledge graph from a chunk to find related code/docs",
                    "inputSchema": {
                        "type": "object",
                        "properties": {
                            "chunk_id": {"type": "string", "description": "Chunk ID to start from"},
                            "relation": {"type": "string", "description": "Filter by relation (imports, calls, documents, etc.)"},
                            "max_depth": {"type": "integer", "description": "Traversal depth", "default": 2},
                        },
                        "required": ["chunk_id"],
                    }
                },
                {
                    "name": "get_chunk",
                    "description": "Get full content of a specific chunk",
                    "inputSchema": {
                        "type": "object",
                        "properties": {
                            "chunk_id": {"type": "string", "description": "Chunk ID"},
                        },
                        "required": ["chunk_id"],
                    }
                },
                {
                    "name": "stats",
                    "description": "Get index statistics",
                    "inputSchema": {
                        "type": "object",
                        "properties": {},
                    }
                },
            ]
        })

    elif method in ("tools/call", "mcp.tools.call"):
        tool_name = params.get("name", "")
        args = params.get("arguments", {})

        if tool_name == "search":
            query = args.get("query", "")
            repo = args.get("repo", "all")
            top_k = args.get("top_k", 10)
            include_graph = args.get("include_graph", True)

            if not query:
                send_error(req_id, -1, "query is required")
                return

            emb = embedder.embed_one(query)
            repo_filter = repo if repo != "all" else None
            results = store.search(emb, top_k=top_k, repo=repo_filter)
            if include_graph:
                for r in results:
                    neighbors = store.get_graph_neighbors(r["chunk_id"], max_depth=1)
                    r["graph_neighbors"] = neighbors[:5]
            send_response(req_id, {"results": results})

        elif tool_name == "graph_walk":
            chunk_id = args.get("chunk_id", "")
            relation = args.get("relation")
            max_depth = args.get("max_depth", 2)
            results = store.get_graph_neighbors(chunk_id, relation=relation, max_depth=max_depth)
            send_response(req_id, {"neighbors": results})

        elif tool_name == "get_chunk":
            chunk_id = args.get("chunk_id", "")
            chunk = store.get_chunk(chunk_id)
            send_response(req_id, {"chunk": chunk})

        elif tool_name == "stats":
            s = store.stats()
            send_response(req_id, {"stats": s})

        else:
            send_error(req_id, -1, f"unknown tool: {tool_name}")

    else:
        send_error(req_id, -1, f"unknown method: {method}")


def main():
    store = Store()
    embedder = Embedder()

    # Send initialized notification
    sys.stderr.write("muxidx MCP server started\n")
    sys.stderr.flush()

    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
            handle_request(req, store, embedder)
        except json.JSONDecodeError:
            sys.stderr.write(f"invalid JSON: {line}\n")
            sys.stderr.flush()
        except Exception as e:
            sys.stderr.write(f"error: {e}\n{traceback.format_exc()}\n")
            sys.stderr.flush()


if __name__ == "__main__":
    main()
