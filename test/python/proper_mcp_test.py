#!/usr/bin/env python3
"""
Proper MCP client test using the official MCP Python library.
This should connect to the Go MCP server using the correct MCP protocol.
"""

import asyncio
import logging
from contextlib import AsyncExitStack

# Official MCP imports
from mcp import ClientSession, StdioServerParameters
from mcp.client.stdio import stdio_client

logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')
logger = logging.getLogger(__name__)

async def test_mcp_connection():
    """Test MCP connection using official client"""
    logger.info("🔗 Testing MCP client connection to Go server...")
    
    try:
        # The Go server uses HTTP streaming, but let's first try to understand
        # what transport it actually provides
        logger.info("Attempting to connect to MCP server...")
        
        # For now, let's test using a different approach - 
        # direct HTTP to the MCP server to see what it responds with
        import httpx
        
        # Test basic connectivity
        async with httpx.AsyncClient() as client:
            try:
                # Try to see what the MCP server responds to
                response = await client.get("http://localhost:8081/")
                logger.info(f"MCP server root response: {response.status_code}")
                logger.info(f"Response body: {response.text[:200]}...")
                
                # Try some MCP-specific paths
                for path in ["/mcp", "/", "/health", "/tools"]:
                    try:
                        response = await client.get(f"http://localhost:8081{path}")
                        logger.info(f"Path {path}: {response.status_code}")
                        if response.status_code == 200:
                            logger.info(f"  Content: {response.text[:100]}...")
                    except Exception as e:
                        logger.info(f"Path {path}: Error - {e}")
                
            except Exception as e:
                logger.error(f"HTTP connection failed: {e}")
                
        return False
        
    except Exception as e:
        logger.error(f"MCP connection test failed: {e}")
        return False

async def test_with_dummy_human():
    """Test the complete flow with dummy human"""
    logger.info("🤖 Testing complete flow with dummy human...")
    
    # Start dummy human in background
    import subprocess
    import os
    
    # Start dummy human script
    dummy_proc = None
    try:
        logger.info("Starting dummy human...")
        dummy_proc = subprocess.Popen([
            "python3", "dummy_human.py"
        ], cwd=os.getcwd())
        
        # Give it time to connect
        await asyncio.sleep(2)
        
        logger.info("Dummy human should be connected, now testing MCP...")
        
        # Now try the MCP connection
        success = await test_mcp_connection()
        
        if success:
            logger.info("✅ End-to-end test with dummy human PASSED!")
        else:
            logger.info("❌ End-to-end test with dummy human FAILED!")
            
        return success
        
    finally:
        if dummy_proc:
            dummy_proc.terminate()
            try:
                dummy_proc.wait(timeout=5)
            except subprocess.TimeoutExpired:
                dummy_proc.kill()
            logger.info("Dummy human process terminated")

async def main():
    """Main test function"""
    logger.info("🧪 Testing MCP Connection with Official Client")
    logger.info("============================================")
    
    # Test 1: Basic MCP connectivity
    logger.info("Test 1: Basic MCP server connectivity")
    conn_success = await test_mcp_connection()
    
    # Test 2: With dummy human
    logger.info("\nTest 2: Complete flow with dummy human")  
    e2e_success = await test_with_dummy_human()
    
    if conn_success or e2e_success:
        logger.info("🎉 Some tests passed!")
        return True
    else:
        logger.error("❌ All tests failed!")
        return False

if __name__ == "__main__":
    success = asyncio.run(main())
    exit(0 if success else 1)