#!/usr/bin/env python3
"""
Dummy human response script that connects to WebSocket server 
and automatically responds with "test" to any ask_user question.
"""

import asyncio
import json
import logging
import websockets
from websockets.exceptions import ConnectionClosedError, ConnectionClosedOK

logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')
logger = logging.getLogger(__name__)

class DummyHuman:
    def __init__(self, ws_url="ws://localhost:8080/ws"):
        self.ws_url = ws_url
        self.websocket = None
        
    async def connect(self):
        """Connect to the WebSocket server"""
        try:
            logger.info(f"Connecting to WebSocket server at {self.ws_url}")
            self.websocket = await websockets.connect(self.ws_url)
            logger.info("Connected to WebSocket server")
            
            # Send connect message
            connect_msg = {"type": "connect"}
            await self.websocket.send(json.dumps(connect_msg))
            logger.info("Sent connect message")
            
        except Exception as e:
            logger.error(f"Failed to connect to WebSocket server: {e}")
            raise
    
    async def listen_and_respond(self):
        """Listen for questions and respond with 'test' automatically"""
        try:
            async for message in self.websocket:
                try:
                    data = json.loads(message)
                    logger.info(f"Received message: {data}")
                    
                    if data.get("type") == "ask_user":
                        request_id = data.get("request_id")
                        question = data.get("question")
                        
                        logger.info(f"Received question: {question}")
                        logger.info(f"Auto-responding with 'test' for request {request_id}")
                        
                        # Send response with "test"
                        response = {
                            "type": "user_response",
                            "request_id": request_id,
                            "answer": "test"
                        }
                        
                        await self.websocket.send(json.dumps(response))
                        logger.info(f"Sent response: test")
                        
                    elif data.get("type") == "connect_ack":
                        logger.info("Received connection acknowledgment")
                        
                except json.JSONDecodeError:
                    logger.error(f"Failed to decode message: {message}")
                except Exception as e:
                    logger.error(f"Error processing message: {e}")
                    
        except (ConnectionClosedError, ConnectionClosedOK):
            logger.info("WebSocket connection closed")
        except Exception as e:
            logger.error(f"Error in listen_and_respond: {e}")
            
    async def run(self):
        """Main run loop"""
        await self.connect()
        await self.listen_and_respond()
        
    async def close(self):
        """Close the WebSocket connection"""
        if self.websocket:
            await self.websocket.close()
            logger.info("WebSocket connection closed")

async def main():
    """Main function"""
    dummy_human = DummyHuman()
    
    try:
        await dummy_human.run()
    except KeyboardInterrupt:
        logger.info("Received interrupt signal")
    except Exception as e:
        logger.error(f"Error in main: {e}")
    finally:
        await dummy_human.close()

if __name__ == "__main__":
    asyncio.run(main())