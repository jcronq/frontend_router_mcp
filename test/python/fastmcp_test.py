#!/usr/bin/env python3
"""
FastMCP client test that connects to the MCP server and tests ask_user tool.
"""

import asyncio
import json
import logging
from typing import Dict, Any
import sys
import time

# Try importing fastmcp - if not available, we'll use a simple HTTP client
try:
    from fastmcp import FastMCP
    FASTMCP_AVAILABLE = True
except ImportError:
    FASTMCP_AVAILABLE = False
    import aiohttp

logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')
logger = logging.getLogger(__name__)

class MCPTestClient:
    def __init__(self, mcp_url="http://localhost:8081"):
        self.mcp_url = mcp_url
        self.client = None
        
    async def connect_fastmcp(self):
        """Connect using FastMCP (if available)"""
        if not FASTMCP_AVAILABLE:
            raise ImportError("FastMCP not available")
            
        logger.info("Connecting to MCP server using FastMCP...")
        # FastMCP may have different initialization API
        self.client = FastMCP(self.mcp_url)
        logger.info("FastMCP client initialized")
        
    async def connect_http(self):
        """Connect using direct HTTP client"""
        logger.info("Using HTTP client for MCP connection...")
        self.client = aiohttp.ClientSession()
        logger.info("HTTP client session created")
        
    async def list_tools_fastmcp(self):
        """List tools using FastMCP"""
        logger.info("Listing available tools via FastMCP...")
        try:
            # Try different possible methods to list tools
            if hasattr(self.client, 'list_tools'):
                tools = await self.client.list_tools()
            elif hasattr(self.client, 'get_tools'):
                tools = await self.client.get_tools()
            else:
                logger.warning("FastMCP list_tools method not found, skipping...")
                return []
            logger.info(f"Available tools: {tools}")
            return tools
        except Exception as e:
            logger.warning(f"FastMCP list_tools failed: {e}, continuing with test...")
            return []
        
    async def list_tools_http(self):
        """List tools using HTTP"""
        logger.info("Listing available tools via HTTP...")
        async with self.client.get(f"{self.mcp_url}/tools") as response:
            if response.status == 200:
                tools = await response.json()
                logger.info(f"Available tools: {tools}")
                return tools
            else:
                logger.error(f"Failed to list tools: HTTP {response.status}")
                return None
                
    async def call_ask_user_fastmcp(self, question: str, timeout: int = 30):
        """Call ask_user tool using FastMCP"""
        logger.info(f"Calling ask_user tool via FastMCP with question: {question}")
        
        try:
            # Try different possible methods to call tools
            if hasattr(self.client, 'call_tool'):
                result = await self.client.call_tool(
                    "ask_user",
                    question=question,
                    timeout_seconds=timeout
                )
            elif hasattr(self.client, 'run_tool'):
                result = await self.client.run_tool(
                    "ask_user",
                    {"question": question, "timeout_seconds": timeout}
                )
            else:
                raise AttributeError("FastMCP call_tool method not found")
                
            logger.info(f"Ask user result: {result}")
            return result
        except Exception as e:
            logger.error(f"FastMCP call_tool failed: {e}")
            raise
        
    async def call_ask_user_http(self, question: str, timeout: int = 30):
        """Call ask_user tool using HTTP"""
        logger.info(f"Calling ask_user tool via HTTP with question: {question}")
        
        request_data = {
            "tool": "ask_user",
            "params": {
                "question": question,
                "timeout_seconds": timeout
            }
        }
        
        async with self.client.post(
            f"{self.mcp_url}/execute",
            json=request_data,
            headers={"Content-Type": "application/json"}
        ) as response:
            if response.status == 200:
                result = await response.text()
                logger.info(f"Ask user result: {result}")
                return result
            else:
                error_text = await response.text()
                logger.error(f"Failed to call ask_user tool: HTTP {response.status}, {error_text}")
                return None
    
    async def run_test(self):
        """Run the complete test"""
        try:
            # Skip FastMCP for now, use HTTP directly
            logger.info("Using HTTP client for MCP testing (FastMCP API incompatible)...")
            
            # HTTP fallback
            if not self.client:
                await self.connect_http()
                await self.list_tools_http()
                
                # Test ask_user tool
                logger.info("Testing ask_user tool...")
                result = await self.call_ask_user_http("What is your favorite color?", 10)
                
                if result and "test" in str(result).lower():
                    logger.info("✅ End-to-end test PASSED! Received expected response.")
                    return True
                else:
                    logger.error(f"❌ Unexpected response: {result}")
                    return False
                    
        except Exception as e:
            logger.error(f"Test failed with error: {e}")
            return False
        
    async def close(self):
        """Close the client connection"""
        if self.client:
            try:
                if hasattr(self.client, 'close'):
                    await self.client.close()
                elif hasattr(self.client, 'disconnect'):
                    await self.client.disconnect()
                elif hasattr(self.client, 'shutdown'):
                    await self.client.shutdown()
            except Exception as e:
                logger.warning(f"Error closing client: {e}")
            logger.info("Client connection closed")

async def main():
    """Main test function"""
    logger.info("🚀 Starting FastMCP end-to-end test...")
    logger.info("Make sure the MCP server is running and dummy_human.py is connected!")
    
    # Give user time to start dummy human
    logger.info("Waiting 2 seconds for dummy human to connect...")
    await asyncio.sleep(2)
    
    client = MCPTestClient()
    
    try:
        success = await client.run_test()
        if success:
            logger.info("🎉 All tests passed! MCP server is working correctly.")
            sys.exit(0)
        else:
            logger.error("💥 Tests failed!")
            sys.exit(1)
            
    except KeyboardInterrupt:
        logger.info("Test interrupted by user")
        sys.exit(1)
    except Exception as e:
        logger.error(f"Test failed with exception: {e}")
        sys.exit(1)
    finally:
        await client.close()

if __name__ == "__main__":
    asyncio.run(main())