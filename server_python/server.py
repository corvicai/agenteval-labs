import asyncio
import time
import httpx
import uuid
import random
from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel
from mcp import ClientSession
from mcp.client.streamable_http import streamablehttp_client
from mcp.client.streamable_http import streamablehttp_client

# Monkey patch httpx AsyncClient to use longer default timeout (10 minutes)
_original_init = httpx.AsyncClient.__init__

def _patched_init(self, *args, **kwargs):
    if 'timeout' not in kwargs:
        kwargs['timeout'] = httpx.Timeout(1600.0, connect=60.0, read=600.0, write=60.0, pool=None)
    _original_init(self, *args, **kwargs)

httpx.AsyncClient.__init__ = _patched_init
print("[INIT] Patched httpx.AsyncClient with 1600s timeout")

app = FastAPI()

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)


class ExecutionRequest(BaseModel):
    request_id: str
    provider_type: str  # 'mcp', 'openai'
    config: dict
    payload: dict


class QueryRequest(BaseModel):
    endpoint: str
    token: str
    question: str


class OpenAIQueryRequest(BaseModel):
    api_key: str
    question: str
    prompt_id: str | None = None
    prompt_version: str | None = None
    image_data: dict | None = None
    original_question: str | None = None
    expected_answer: str | None = None


async def run_mcp_query(endpoint: str, token: str, question: str):
    print(f"[PYTHON] Running MCP query for: {question[:50]}...")
    
    # DRYRUN/MOCK MODE: Simulate MCP responses without real connection
    if token in ["DRYRUN", "MOCK", "MOCK_TOKEN"]:
        print("[PYTHON] 🎭 MOCK MCP MODE DETECTED")
        start_time = time.time()
        delay = random.uniform(0.5, 2.0)
        await asyncio.sleep(delay)
        
        MOCK_ANSWERS = [
            "The answer is correct according to the logic provided.",
            "Tokyo is indeed the capital city of Japan.",
            "The pills will last exactly one hour (0m, 30m, 60m).",
            "Leonardo da Vinci completed the Mona Lisa.",
            "299,792,458 meters per second in a vacuum.",
            "Compound interest generates exponential growth.",
            "Kubernetes manages containers across clusters.",
            "Yes, following the transitive property (A -> B -> C).",
            "George Orwell wrote 1984 in 1949.",
            "Au comes from the Latin word Aurum.",
            "TCP ensures delivery, UDP sends packets without verification.",
            "Binary search cuts the search space in half each step.",
            "Deep Learning is a subset of Machine Learning using neural networks.",
            "The Trolley Problem highlights utilitarian vs deontological ethics.",
            "Red leaves falling down / Gold and crunch under my feet / Winter is waking.",
            "The Red Bean Roastery.",
            "It smells like wet asphalt and fresh soil.",
            "To measure 4L: Fill 5, pour to 3. 2 left in 5. Empty 3. Pour 2 to 3. Fill 5. Pour 1 to 3. 4 left.",
            "O(n^2) is the worst case for bubble sort.",
            "HTTP is stateless, HTTPS is secure.",
            "AI is the simulation of human intelligence in machines.",
            "Machine learning allows computers to learn from data without explicit programming.",
            "A neural network mimics the brain's structure with layers of connected nodes.",
            "Backpropagation adjusts weights by propagating errors backward through the network.",
            "Docker containerizes applications for consistent deployment across environments.",
            "AI bias occurs when training data reflects societal prejudices.",
            "Alignment ensures AI systems act according to human values and intentions.",
        ]
        
        mock_answer = random.choice(MOCK_ANSWERS)
        total_duration = time.time() - start_time
        
        return {
            "success": True,
            "answer": mock_answer,
            "metadata": {
                "duration_ms": int(total_duration * 1000),
                "raw_response": {
                    "content": [{"type": "text", "text": mock_answer}],
                    "isError": False
                }
            }
        }

    # REAL MODE: Normal MCP connection
    start_time = time.time()
    try:
        async with streamablehttp_client(
            endpoint,
            headers={
                "Content-Type": "application/json",
                "Accept": "application/json, text/event-stream",
                "Authorization": str(token or ""),
            },
        ) as (read, write, session_id):
            async with ClientSession(read, write) as session:
                await session.initialize()
                
                call_start = time.time()
                # Simple progress logging
                async def log_progress():
                    while True:
                        await asyncio.sleep(10)
                        print(f"[PYTHON] Still processing MCP... {int(time.time() - call_start)}s")
                
                progress_task = asyncio.create_task(log_progress())
                try:
                    result = await session.call_tool(
                        "query",
                        arguments={"query_content": question},
                    )
                finally:
                    progress_task.cancel()
                    try:
                        await progress_task
                    except asyncio.CancelledError:
                        pass

                total_duration = time.time() - start_time
                result_dict = result.model_dump() if hasattr(result, 'model_dump') else dict(result)
                
                def extract_text_from_item(item):
                    if not isinstance(item, dict):
                        if isinstance(item, str):
                            return item
                        return ""

                    item_type = item.get("type")
                    if item_type == "text":
                        text_val = item.get("text")
                        if isinstance(text_val, str):
                            return text_val
                        if isinstance(text_val, dict):
                            return text_val.get("text") or text_val.get("value") or ""

                    text_val = item.get("text")
                    if isinstance(text_val, str):
                        return text_val
                    if isinstance(text_val, dict):
                        return text_val.get("text") or text_val.get("value") or ""

                    content_val = item.get("content")
                    if isinstance(content_val, str):
                        return content_val

                    resource_val = item.get("resource")
                    if isinstance(resource_val, dict):
                        res_text = resource_val.get("text") or resource_val.get("data")
                        if isinstance(res_text, str):
                            return res_text

                    return ""

                answer_text = ""
                content = result_dict.get("content")
                if isinstance(content, list):
                    for item in content:
                        answer_text += extract_text_from_item(item)
                elif content is not None:
                    answer_text = extract_text_from_item(content)

                if not answer_text:
                    for key in ("result", "output", "message"):
                        val = result_dict.get(key)
                        if isinstance(val, str) and val.strip():
                            answer_text = val
                            break

                if not answer_text:
                    answer_text = str(result_dict)
                
                return {
                    "success": True, 
                    "answer": answer_text,
                    "metadata": {
                        "duration_ms": int(total_duration * 1000),
                        "raw_response": result_dict
                    }
                }
    except ExceptionGroup as eg:
        # Python 3.11+ TaskGroup errors
        print(f"[PYTHON] ❌ MCP ExceptionGroup ERROR: {eg}")
        # Extract first sub-exception for cleaner message
        first_error = eg.exceptions[0] if eg.exceptions else eg
        return {
            "success": False,
            "error": str(first_error),
            "metadata": {"duration_ms": int((time.time() - start_time) * 1000)}
        }
    except Exception as e:
        print(f"[PYTHON] ❌ MCP HTTP ERROR: {e}")
        return {
            "success": False,
            "error": str(e),
            "metadata": {"duration_ms": int((time.time() - start_time) * 1000)}
        }





async def run_openai_query(
    api_key: str, 
    question: str, 
    prompt_id: str = None, 
    prompt_version: str = None, 
    image_data: dict = None,
    original_question: str = None,
    expected_answer: str = None
):
    print(f"[PYTHON] Running OpenAI query for: {question[:50]}...")
    start_time = time.time()
    
    # MOCK MODE: Handle MOCK or DRYRUN tokens if explicitly requested
    if api_key and ("MOCK" in api_key.upper() or "DRYRUN" in api_key.upper()):
        print("[PYTHON] 🎭 MOCK/DRYRUN OPENAI MODE DETECTED")

        await asyncio.sleep(random.uniform(0.5, 1.5))
        
        # Simple mock evaluation logic
        mock_answer = "Evaluation complete (MOCK): The agent response appears accurate and follow instructions."
        if "answer" in question.lower() or "expected" in question.lower():
            mock_answer = "PASS (MOCK): The target agent's response correctly matches the essence of the expected answer."
        elif "evalu" in question.lower():
            mock_answer = "Rating (MOCK): 5/5. Reason: The response is helpful, clear, and concise."
            
        total_duration = time.time() - start_time
        return {
            "success": True,
            "answer": mock_answer,
            "metadata": {
                "duration_ms": int(total_duration * 1000),
                "raw_response": {"mock": True, "status": "simulated"}
            }
        }
    
    # Validation for real Mode
    if not api_key:
        return {
            "success": False,
            "error": "OpenAI API Key is missing. Please configure your Evaluator Agent or use 'MOCK' as the key.",
            "metadata": {"duration_ms": 0}
        }



    try:
        from openai import AsyncOpenAI
        client = AsyncOpenAI(api_key=api_key)
        
        has_image = image_data is not None

        if prompt_id and prompt_version:
            print(f"[PYTHON] ✅ Using PLATFORM PROMPT")
            print(f"[PYTHON] ✅ Using PLATFORM PROMPT")
            print(f"[PYTHON] DEBUG: Incoming question (Response to Evaluate): '{question}'")
            print(f"[PYTHON] DEBUG: Original Question: '{original_question}'")
            print(f"[PYTHON] DEBUG: Expected Answer: '{expected_answer}'")
            
            if has_image:
                image = image_data or {}
                mime = image.get("content_type") or "image/png"
                base64_data = image.get("base64_data")
                if not base64_data:
                    raise ValueError("image_data.base64_data is required when image_data is provided")
                data_url = f"data:{mime};base64,{base64_data}"
                input_payload = [{"role": "user", "content": [{"type": "input_image", "image_url": data_url}]}]
            else:
                # Inject context into the input payload for the template
                if original_question or expected_answer:
                    input_text = "EVALUATION TASK\n\n"
                    if original_question:
                        input_text += f"**Original Question:**\n{original_question}\n\n"
                    if expected_answer:
                        input_text += f"**Expected Answer (Gabarito):**\n{expected_answer}\n\n"
                    input_text += f"**Response to Evaluate:**\n{question}"
                    input_payload = input_text
                else:
                    input_payload = question
                
                print(f"[PYTHON] DEBUG: Final input_payload len: {len(str(input_payload))}")
                print(f"[PYTHON] DEBUG: Final input_payload content: {str(input_payload)}")

            response = await client.responses.create(
                prompt={"id": prompt_id, "version": prompt_version},
                input=input_payload,
                reasoning={},
                store=True,
                include=["web_search_call.action.sources"],
            )

            result_text = None
            try:
                if hasattr(response, "output_text") and response.output_text:
                    result_text = response.output_text
                elif hasattr(response, "output") and response.output:
                    first_output = response.output[0]
                    content_list = getattr(first_output, "content", None) or []
                    if content_list:
                        first_content = content_list[0]
                        text_attr = getattr(first_content, "text", None)
                        if text_attr:
                            result_text = text_attr
            except Exception:
                pass
            if not result_text:
                result_text = str(response)

        elif has_image:
            print("[PYTHON] ⚠️ FALLBACK MODE: Image provided but no prompt configured")
            image = image_data or {}
            mime = image.get("content_type") or "image/png"
            base64_data = image.get("base64_data")
            if not base64_data:
                raise ValueError("image_data.base64_data is required when image_data is provided")
            data_url = f"data:{mime};base64,{base64_data}"
            messages = [{"role": "user", "content": [{"type": "image_url", "image_url": {"url": data_url}}]}]
            response = await client.chat.completions.create(model="gpt-4o-mini", messages=messages)
            result_text = str(response.choices[0].message.content)
        else:
            print(f"[PYTHON] ⚠️ FALLBACK MODE: No prompt_id configured")
            max_chars = 50000
            
            # Build an enhanced evaluation prompt if we have context
            if original_question or expected_answer:
                prompt = "EVALUATION TASK\n\n"
                if original_question:
                    prompt += f"**Original Question:**\n{original_question}\n\n"
                if expected_answer:
                    prompt += f"**Expected Answer (Gabarito):**\n{expected_answer}\n\n"
                prompt += f"**Response to Evaluate:**\n{question}\n\n"
                prompt += "Please evaluate if the response correctly addresses the original question and matches the expected answer if provided. If the question is about one topic (e.g. apples) but the response is about another (e.g. bananas), mark it as a failure."
                question_text = prompt[:max_chars]
            else:
                question_text = question[:max_chars]

            response = await client.chat.completions.create(
                model="gpt-4o-mini",
                messages=[{"role": "user", "content": question_text}]
            )
            result_text = response.choices[0].message.content

        total_duration = time.time() - start_time
        return {
            "success": True,
            "answer": result_text,
            "metadata": {
                "duration_ms": int(total_duration * 1000),
                "raw_response": str(response)
            }
        }
    except Exception as e:
        print(f"[PYTHON] ❌ OpenAI ERROR: {e}")
        return {
            "success": False,
            "error": str(e),
            "metadata": {"duration_ms": int((time.time() - start_time) * 1000)}
        }


@app.post("/execute")
async def execute(request: ExecutionRequest):
    print(f"\n[PYTHON] Received Execution Request: {request.request_id} ({request.provider_type})")
    
    if request.provider_type == "mcp":
        config = request.config
        payload = request.payload
        
        result = await run_mcp_query(
            endpoint=config.get("endpoint"),
            token=str(config.get("token") or ""),
            question=payload.get("question")
        )
    elif request.provider_type == "openai":
        config = request.config
        payload = request.payload
        result = await run_openai_query(
            api_key=str(config.get("api_key") or ""),
            question=payload.get("question"),
            prompt_id=config.get("prompt_id"),
            prompt_version=config.get("prompt_version"),
            image_data=payload.get("image_data"),
            original_question=payload.get("original_question"),
            expected_answer=payload.get("expected_answer")
        )
    else:
        raise HTTPException(status_code=400, detail=f"Unknown provider type: {request.provider_type}")

    return {
        "request_id": request.request_id,
        "success": result.get("success"),
        "answer": result.get("answer"),
        "error": result.get("error"),
        "metadata": result.get("metadata")
    }


@app.post("/query")
async def legacy_query(request: QueryRequest):
    res = await run_mcp_query(request.endpoint, request.token, request.question)
    return {
        "success": res["success"],
        "answer": res["answer"],
        "duration": str(res["metadata"]["duration_ms"] / 1000.0),
        "result": res["metadata"]["raw_response"]
    }


@app.post("/query-openai")
async def legacy_query_openai(request: OpenAIQueryRequest):
    res = await run_openai_query(
        request.api_key, 
        request.question, 
        request.prompt_id, 
        request.prompt_version, 
        request.image_data,
        request.original_question,
        request.expected_answer
    )
    return {
        "success": res["success"],
        "answer": res["answer"],
        "duration": str(res["metadata"]["duration_ms"] / 1000.0),
        "result": {"content": [{"type": "text", "text": res["answer"]}]}
    }


@app.get("/health")
async def health():
    return {"status": "ok"}


if __name__ == "__main__":
    import uvicorn
    import os
    port = int(os.environ.get("PORT", 3003))
    print("=" * 60)
    print("PYTHON MCP AGENT SERVER")
    print(f"Starting on port {port}...")
    print("=" * 60)
    uvicorn.run(app, host="0.0.0.0", port=port, log_level="info")
