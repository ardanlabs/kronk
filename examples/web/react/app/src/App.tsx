import { useState, useRef, useEffect } from 'react'
import './App.css'

interface ToolCall {
  id: string
  name: string
  arguments: Record<string, unknown>
}

interface Message {
  role: string
  content: string
  reasoning?: string
  toolCall?: ToolCall
  usage?: Usage
}

interface Usage {
  input_tokens: number
  reasoning_tokens: number
  completion_tokens: number
  output_tokens: number
  tokens_per_second: number
}

interface ChatRequest {
  stream: boolean
  messages: Message[]
  temperature?: number
  top_p?: number
  top_k?: number
  max_tokens?: number
}

interface Choice {
  index: number
  delta: {
    role?: string
    content?: string
    reasoning?: string
    tool_calls?: ToolCall[]
  }
  finish_reason?: string
  generated_text?: string
  GeneratedText?: string
}

interface Response {
  id?: string
  created?: number
  model?: string
  choices?: Choice[]
  error?: string
  usage?: Usage
}

function App() {
  const [messages, setMessages] = useState<Message[]>([])
  const [input, setInput] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [temperature, setTemperature] = useState(1.0)
  const [topP, setTopP] = useState(0.95)
  const [topK, setTopK] = useState(50)
  const [maxTokens, setMaxTokens] = useState(1000)
  const messagesEndRef = useRef<HTMLDivElement>(null)

  const scrollToBottom = () => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }

  useEffect(() => {
    scrollToBottom()
  }, [messages])

  const sendMessage = async () => {
    if (!input.trim() || isLoading) return

    const userMessage: Message = {
      role: 'user',
      content: input.trim()
    }

    const updatedMessages = [...messages, userMessage]
    setMessages(updatedMessages)
    setInput('')
    setIsLoading(true)

    const assistantMessage: Message = {
      role: 'assistant',
      content: ''
    }
    setMessages([...updatedMessages, assistantMessage])

    try {
        const requestBody: ChatRequest = {
        stream: true,
        messages: updatedMessages,
        temperature,
        top_p: topP,
        top_k: topK,
        max_tokens: maxTokens
      }

      const response = await fetch('http://localhost:8080/chat', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify(requestBody)
      })

      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`)
      }

      const reader = response.body?.getReader()
      const decoder = new TextDecoder()

      if (!reader) {
        throw new Error('No response body')
      }

      let accumulatedContent = ''
      let accumulatedReasoning = ''
      let toolCall: ToolCall | undefined = undefined

      while (true) {
        const { done, value } = await reader.read()
        if (done) break

        const chunk = decoder.decode(value, { stream: true })
        const lines = chunk.split('\n')

        for (const line of lines) {
          if (line.startsWith('data: ')) {
            const data = line.substring(6)
            
            if (data === '[DONE]') {
              break
            }
            
            try {
              const parsed: Response = JSON.parse(data)
              
              if (parsed.error) {
                console.error('Error from server:', parsed.error)
                accumulatedContent += `\n[Error: ${parsed.error}]`
              } else if (parsed.choices && parsed.choices.length > 0) {
                const choice = parsed.choices[0]

                if (choice.finish_reason === 'error') {
                  const errorText = choice.GeneratedText || choice.generated_text || 'Unknown Error'
                  accumulatedContent += `\n[Error: ${errorText}]`
                } else if (choice.finish_reason === 'tool_calls') {
                  if (choice.delta?.tool_calls && choice.delta.tool_calls.length > 0) {
                    toolCall = choice.delta.tool_calls[0]
                  }
                } else {
                  if (choice.finish_reason !== 'stop') {
                    if (choice.delta?.content) {
                      accumulatedContent += choice.delta.content
                    }
                    if (choice.delta?.reasoning) {
                      accumulatedReasoning += choice.delta.reasoning
                    }
                  }
                }
              }

              setMessages(prev => {
                const newMessages = [...prev]
                const currentMsg = newMessages[newMessages.length - 1]
                newMessages[newMessages.length - 1] = {
                  role: 'assistant',
                  content: accumulatedContent,
                  reasoning: accumulatedReasoning,
                  toolCall: toolCall || currentMsg.toolCall,
                  usage: parsed.usage || currentMsg.usage
                }
                return newMessages
              })
            } catch (e) {
              console.error('Failed to parse JSON:', e)
            }
          }
        }
      }
    } catch (error) {
      console.error('Error sending message:', error)
      setMessages(prev => {
        const newMessages = [...prev]
        newMessages[newMessages.length - 1] = {
          role: 'assistant',
          content: 'Error: Failed to get response from server'
        }
        return newMessages
      })
    } finally {
      setIsLoading(false)
    }
  }

  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      sendMessage()
    }
  }

  const clearHistory = () => {
    setMessages([])
  }

  return (
    <div className="app">
      <div className="chat-container">
        <div className="parameters">
          <div className="param">
            <label>Temperature: {temperature}</label>
            <input
              type="range"
              min="0"
              max="2"
              step="0.1"
              value={temperature}
              onChange={(e) => setTemperature(parseFloat(e.target.value))}
              disabled={isLoading}
            />
          </div>
          <div className="param">
            <label>Top P: {topP}</label>
            <input
              type="range"
              min="0"
              max="1"
              step="0.05"
              value={topP}
              onChange={(e) => setTopP(parseFloat(e.target.value))}
              disabled={isLoading}
            />
          </div>
          <div className="param">
            <label>Top K: {topK}</label>
            <input
              type="range"
              min="1"
              max="100"
              step="1"
              value={topK}
              onChange={(e) => setTopK(parseInt(e.target.value))}
              disabled={isLoading}
            />
          </div>
          <div className="param">
            <label>Max Tokens: {maxTokens}</label>
            <input
              type="range"
              min="10"
              max="4000"
              step="10"
              value={maxTokens}
              onChange={(e) => setMaxTokens(parseInt(e.target.value))}
              disabled={isLoading}
            />
          </div>
          <button onClick={clearHistory} className="clear-button" disabled={isLoading}>
            Clear History
          </button>
        </div>
        <div className="messages">
          {messages.map((msg, idx) => (
            <div key={idx} className={`message ${msg.role}`}>
              <div className="role">{msg.role === 'user' ? 'You' : 'Assistant'}</div>
              <div className="content">
                {msg.reasoning && (
                  <div style={{ color: 'red', whiteSpace: 'pre-wrap', marginBottom: '10px' }}>
                    {msg.reasoning}
                  </div>
                )}
                {msg.content}
                {msg.toolCall && (
                  <div style={{ color: '#006400', whiteSpace: 'pre-wrap', marginTop: '10px', fontFamily: 'monospace' }}>
                    Model Asking For Tool Call:<br />
                    ToolID[{msg.toolCall.id}]: {msg.toolCall.name}({JSON.stringify(msg.toolCall.arguments)})
                  </div>
                )}
                {msg.usage && (
                  <div className="usage-info" style={{ fontSize: '0.8em', color: '#888', marginTop: '8px', paddingTop: '8px', borderTop: '1px solid #eee' }}>
                    Input: {msg.usage.input_tokens} | Output: {msg.usage.output_tokens} | Speed: {msg.usage.tokens_per_second.toFixed(2)} t/s
                  </div>
                )}
              </div>
            </div>
          ))}
          <div ref={messagesEndRef} />
        </div>
        <div className="input-container">
          <input
            type="text"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyPress={handleKeyPress}
            placeholder="Type your message..."
            disabled={isLoading}
          />
          <button onClick={sendMessage} disabled={isLoading || !input.trim()}>
            {isLoading ? 'Sending...' : 'Send'}
          </button>
        </div>
      </div>
    </div>
  )
}

export default App
