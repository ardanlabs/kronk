import { api } from './api'
import type {
  ChatStreamResponse,
  ChatToolCall,
  ChatUsage,
  AutoTestPromptDef,
  AutoTestScenario,
  AutoTestScenarioID,
  AutoTestScenarioResult,
  AutoTestPromptResult,
  AutoTestTrialResult,
  SamplingCandidate,
} from '../types'

/** Standard tool definitions used for automated testing. */
export const autoTestTools = [
  {
    type: 'function',
    function: {
      name: 'get_weather',
      description: 'Get current weather for a city',
      parameters: {
        type: 'object',
        properties: {
          location: { type: 'string', description: 'City name' },
          unit: { type: 'string', enum: ['celsius', 'fahrenheit'] },
        },
        required: ['location'],
      },
    },
  },
  {
    type: 'function',
    function: {
      name: 'add',
      description: 'Add two numbers',
      parameters: {
        type: 'object',
        properties: {
          a: { type: 'number' },
          b: { type: 'number' },
        },
        required: ['a', 'b'],
      },
    },
  },
]

/** Chat scenario — tests basic text generation quality. */
export const chatScenario: AutoTestScenario = {
  id: 'chat',
  name: 'Chat Quality',
  prompts: [
    {
      id: 'math-multiply',
      messages: [{ role: 'user', content: 'What is 17 * 19? Answer with only the number.' }],
      expected: { type: 'exact', value: '323' },
    },
    {
      id: 'list-benefits',
      messages: [{ role: 'user', content: 'List exactly 3 benefits of exercise. Use a numbered list.' }],
      expected: { type: 'regex', value: '^\\s*1[.)]\\s+.+\\n\\s*2[.)]\\s+.+\\n\\s*3[.)]\\s+.+' },
    },
    {
      id: 'translate-french',
      messages: [{ role: 'user', content: "Translate 'Good morning' to French. Answer with only the translation." }],
      expected: { type: 'regex', value: 'bonjour' },
    },
  ],
}

/** Tool call scenario — tests tool/function calling capability. */
export const toolCallScenario: AutoTestScenario = {
  id: 'tool_call',
  name: 'Tool Calling',
  prompts: [
    {
      id: 'weather-tool',
      messages: [{ role: 'user', content: "What's the weather in Boston?" }],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'add-tool',
      messages: [{ role: 'user', content: 'What is 15 + 28?' }],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
  ],
}

/** Generates trial candidates with expanded parameter grids, truncated to maxTrials. */
export function generateTrialCandidates(
  baseline: SamplingCandidate,
  maxTrials: number = 25,
): SamplingCandidate[] {
  const safeMax = Number.isFinite(maxTrials) ? Math.max(1, Math.floor(maxTrials)) : 25

  const base: SamplingCandidate = {
    temperature: 0.8,
    top_p: 0.9,
    top_k: 40,
    min_p: 0,
    ...baseline,
  }

  // Normalize floats to 3 decimal places for stable comparison and dedup
  const norm = (n: number | undefined) =>
    n !== undefined && Number.isFinite(n) ? Math.round(n * 1000) / 1000 : n

  const seen = new Set<string>()
  const candidates: SamplingCandidate[] = []

  const keyOf = (c: SamplingCandidate) =>
    `t=${norm(c.temperature)}|p=${norm(c.top_p)}|k=${c.top_k}|m=${norm(c.min_p)}`

  const add = (c: SamplingCandidate) => {
    if (candidates.length >= safeMax) return
    const k = keyOf(c)
    if (seen.has(k)) return
    seen.add(k)
    candidates.push(c)
  }

  const approxEq = (a: number | undefined, b: number) =>
    a !== undefined && Math.abs(a - b) < 0.001

  // 1) Baseline always first
  add({ ...base })
  if (safeMax <= 1) return candidates

  // Expanded grids
  const temperatureGrid = [0.0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0, 1.1, 1.2]
  const topPGrid = [0.50, 0.55, 0.60, 0.65, 0.70, 0.75, 0.80, 0.85, 0.90, 0.95, 1.00]
  const topKGrid = [0, 5, 10, 20, 30, 40, 60, 80, 120, 200]
  const minPGrid = [0, 0.01, 0.02, 0.03, 0.05, 0.08, 0.10, 0.12, 0.15, 0.20]

  // Sort each grid by distance from baseline value (closest first for early coverage)
  const sortByDistance = (vals: number[], center: number) =>
    [...vals].sort((a, b) => Math.abs(a - center) - Math.abs(b - center))

  const baseTemp = base.temperature ?? 0.8
  const baseTopP = base.top_p ?? 0.9
  const baseTopK = base.top_k ?? 40
  const baseMinP = base.min_p ?? 0

  const temps = sortByDistance(
    temperatureGrid.filter(t => !approxEq(base.temperature, t)),
    baseTemp,
  ).map(t => ({ ...base, temperature: t }))

  const topPs = sortByDistance(
    topPGrid.filter(p => !approxEq(base.top_p, p)),
    baseTopP,
  ).map(p => ({ ...base, top_p: p }))

  const topKs = sortByDistance(
    topKGrid.filter(k => k !== baseTopK),
    baseTopK,
  ).map(k => ({ ...base, top_k: k }))

  const minPs = sortByDistance(
    minPGrid.filter(m => !approxEq(base.min_p, m)),
    baseMinP,
  ).map(m => ({ ...base, min_p: m }))

  // 2) Round-robin OAT interleave across parameters
  const oatLists = [temps, topPs, topKs, minPs]
  for (let i = 0; candidates.length < safeMax; i++) {
    let addedAny = false
    for (const list of oatLists) {
      if (i < list.length) {
        add(list[i])
        addedAny = true
        if (candidates.length >= safeMax) break
      }
    }
    if (!addedAny) break
  }

  // 3) Multi-parameter presets
  const presets: SamplingCandidate[] = [
    { ...base, temperature: 0.2, top_p: 0.70, top_k: 20, min_p: 0.00 },
    { ...base, temperature: 0.6, top_p: 0.90, top_k: 40, min_p: 0.02 },
    { ...base, temperature: 1.0, top_p: 0.95, top_k: 0, min_p: 0.00 },
    { ...base, temperature: 0.4, top_p: 0.80, top_k: 30, min_p: 0.05 },
    { ...base, temperature: 0.8, top_p: 0.85, top_k: 60, min_p: 0.03 },
  ]
  presets.forEach(add)

  // 4) Pairwise corner combos for interaction discovery
  const tLow = temperatureGrid[0]
  const tHigh = temperatureGrid[temperatureGrid.length - 1]
  const pLow = topPGrid[0]
  const pHigh = topPGrid[topPGrid.length - 1]
  const kLow = topKGrid[0]
  const kHigh = topKGrid[topKGrid.length - 1]
  const mHigh = minPGrid[minPGrid.length - 1]

  const corners: SamplingCandidate[] = [
    { ...base, temperature: tLow, top_p: pLow },
    { ...base, temperature: tLow, top_p: pHigh },
    { ...base, temperature: tHigh, top_p: pLow },
    { ...base, temperature: tHigh, top_p: pHigh },
    { ...base, temperature: tLow, top_k: kLow },
    { ...base, temperature: tLow, top_k: kHigh },
    { ...base, temperature: tHigh, top_k: kLow },
    { ...base, temperature: tHigh, top_k: kHigh },
    { ...base, top_p: pLow, top_k: kLow },
    { ...base, top_p: pLow, top_k: kHigh },
    { ...base, top_p: pHigh, top_k: kLow },
    { ...base, top_p: pHigh, top_k: kHigh },
    { ...base, min_p: mHigh, temperature: tLow },
    { ...base, min_p: mHigh, temperature: tHigh },
    { ...base, min_p: mHigh, top_p: pLow },
    { ...base, min_p: mHigh, top_p: pHigh },
  ]
  corners.forEach(add)

  return candidates
}

/** Scores chat text output against an expected value. */
export function scoreChat(
  text: string,
  expected: AutoTestPromptDef['expected'],
): { score: number; notes: string[] } {
  const notes: string[] = []
  let score = 0

  if (!expected) {
    return { score: 0, notes: ['No expected value defined'] }
  }

  switch (expected.type) {
    case 'exact': {
      const trimmed = text.trim().toLowerCase()
      const target = (expected.value ?? '').trim().toLowerCase()
      if (trimmed === target) {
        score = 100
      } else if (trimmed.includes(target)) {
        score = 50
        notes.push(`Partial match: response contains "${expected.value}" but has extra text`)
      } else {
        score = 0
        notes.push(`Expected "${expected.value}", got "${text.trim().slice(0, 100)}"`)
      }
      break
    }
    case 'regex': {
      const re = new RegExp(expected.value ?? '', 'im')
      if (re.test(text)) {
        score = 100
      } else {
        score = 0
        notes.push(`Regex /${expected.value}/ did not match`)
      }
      break
    }
    case 'tool_call': {
      score = 0
      notes.push('tool_call expected type not applicable for chat scoring')
      break
    }
  }

  if (text.length > 2000) {
    score = Math.max(0, score - 10)
    notes.push('Penalized: response exceeds 2000 characters')
  }

  return { score, notes }
}

/** Scores tool call output against declared tools. */
export function scoreToolCall(
  toolCalls: ChatToolCall[],
  tools: any[],
): { score: number; notes: string[] } {
  const notes: string[] = []

  if (!toolCalls || toolCalls.length === 0) {
    return { score: 0, notes: ['No tool calls emitted'] }
  }

  let score = 100

  const declaredNames = new Set(
    tools
      .filter((t: any) => t.type === 'function' && t.function?.name)
      .map((t: any) => t.function.name),
  )

  for (const tc of toolCalls) {
    if (!declaredNames.has(tc.function.name)) {
      score -= 40
      notes.push(`Tool "${tc.function.name}" not in declared tools`)
    }

    try {
      const args = JSON.parse(tc.function.arguments)
      const toolDef = tools.find(
        (t: any) => t.type === 'function' && t.function?.name === tc.function.name,
      )
      if (toolDef?.function?.parameters?.required) {
        const required: string[] = toolDef.function.parameters.required
        for (const field of required) {
          if (!(field in args)) {
            score -= 20
            notes.push(`Missing required field "${field}" in ${tc.function.name}`)
          }
        }
      }
    } catch {
      score -= 30
      notes.push(`Arguments for "${tc.function.name}" are not valid JSON`)
    }
  }

  return { score: Math.max(0, Math.min(100, score)), notes }
}

/** Runs a single prompt against the playground session and scores the result. */
export function runSinglePrompt(
  sessionId: string,
  prompt: AutoTestPromptDef,
  candidate: SamplingCandidate,
): Promise<AutoTestPromptResult> {
  return new Promise((resolve, reject) => {
    let fullContent = ''
    const collectedToolCalls: ChatToolCall[] = []
    let usage: ChatUsage | undefined

    api.streamPlaygroundChat(
      {
        session_id: sessionId,
        messages: prompt.messages,
        tools: prompt.tools,
        stream: true,
        temperature: candidate.temperature,
        top_p: candidate.top_p,
        top_k: candidate.top_k,
        min_p: candidate.min_p,
        max_tokens: candidate.max_tokens ?? prompt.max_tokens,
        stream_options: { include_usage: true },
      } as any,
      (data: ChatStreamResponse) => {
        const choice = data.choices?.[0]
        if (choice?.delta?.content) {
          fullContent += choice.delta.content
        }
        if (choice?.delta?.tool_calls) {
          for (const tc of choice.delta.tool_calls) {
            const existing = collectedToolCalls.find(c => c.index === tc.index)
            if (existing) {
              if (tc.function?.arguments) {
                existing.function.arguments += tc.function.arguments
              }
            } else {
              collectedToolCalls.push({
                id: tc.id || '',
                index: tc.index,
                type: tc.type || 'function',
                function: {
                  name: tc.function?.name || '',
                  arguments: tc.function?.arguments || '',
                },
              })
            }
          }
        }
        if (data.usage) {
          usage = data.usage
        }
      },
      (error: string) => {
        reject(new Error(error))
      },
      () => {
        let scored: { score: number; notes: string[] }

        if (prompt.expected?.type === 'tool_call') {
          scored = scoreToolCall(collectedToolCalls, prompt.tools ?? [])
        } else {
          scored = scoreChat(fullContent, prompt.expected)
        }

        resolve({
          promptId: prompt.id,
          assistantText: fullContent,
          toolCalls: collectedToolCalls,
          usage,
          score: scored.score,
          notes: scored.notes.length > 0 ? scored.notes : undefined,
        })
      },
    )
  })
}

/** Probes whether the current session/template supports tool calling. */
export async function probeTemplate(sessionId: string): Promise<boolean> {
  const prompt: AutoTestPromptDef = {
    id: 'probe-tool',
    messages: [{ role: 'user', content: "What's the weather in Boston?" }],
    tools: autoTestTools,
    expected: { type: 'tool_call' },
  }

  try {
    const result = await runSinglePrompt(sessionId, prompt, {})
    return result.toolCalls.length > 0 && result.score > 0
  } catch {
    return false
  }
}

/** Runs a full trial for one sampling candidate across all scenarios. */
export async function runTrial(
  sessionId: string,
  candidate: SamplingCandidate,
  scenarios: AutoTestScenario[],
  onUpdate: (result: AutoTestTrialResult) => void,
  signal: AbortSignal,
): Promise<AutoTestTrialResult> {
  const trialId = `trial-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`

  const result: AutoTestTrialResult = {
    id: trialId,
    status: 'running',
    candidate,
    startedAt: new Date().toISOString(),
    scenarioResults: [],
  }

  onUpdate({ ...result })

  for (const scenario of scenarios) {
    if (signal.aborted) {
      result.status = 'cancelled'
      onUpdate({ ...result })
      return result
    }

    const promptResults: AutoTestPromptResult[] = []
    let totalTPS = 0
    let tpsCount = 0

    for (const prompt of scenario.prompts) {
      if (signal.aborted) {
        result.status = 'cancelled'
        onUpdate({ ...result })
        return result
      }

      try {
        const pr = await runSinglePrompt(sessionId, prompt, candidate)
        promptResults.push(pr)
        if (pr.usage?.tokens_per_second) {
          totalTPS += pr.usage.tokens_per_second
          tpsCount++
        }
      } catch (err) {
        promptResults.push({
          promptId: prompt.id,
          assistantText: '',
          toolCalls: [],
          score: 0,
          notes: [`Error: ${err instanceof Error ? err.message : String(err)}`],
        })
      }

      const scenarioResult: AutoTestScenarioResult = {
        scenarioId: scenario.id,
        promptResults: [...promptResults],
        score: promptResults.reduce((sum, r) => sum + r.score, 0) / promptResults.length,
        avgTPS: tpsCount > 0 ? totalTPS / tpsCount : undefined,
      }

      const existingIdx = result.scenarioResults.findIndex(s => s.scenarioId === scenario.id)
      if (existingIdx >= 0) {
        result.scenarioResults[existingIdx] = scenarioResult
      } else {
        result.scenarioResults.push(scenarioResult)
      }

      onUpdate({ ...result, scenarioResults: [...result.scenarioResults] })
    }
  }

  const scoreByScenario = new Map<AutoTestScenarioID, number>()
  let totalTrialTPS = 0
  let trialTPSCount = 0

  for (const sr of result.scenarioResults) {
    scoreByScenario.set(sr.scenarioId, sr.score)
    if (sr.avgTPS) {
      totalTrialTPS += sr.avgTPS
      trialTPSCount++
    }
  }

  const toolScore = scoreByScenario.get('tool_call')
  const chatScore = scoreByScenario.get('chat')

  if (toolScore !== undefined && chatScore !== undefined) {
    result.totalScore = 0.6 * toolScore + 0.4 * chatScore
  } else if (toolScore !== undefined) {
    result.totalScore = toolScore
  } else if (chatScore !== undefined) {
    result.totalScore = chatScore
  }

  result.avgTPS = trialTPSCount > 0 ? totalTrialTPS / trialTPSCount : undefined
  result.finishedAt = new Date().toISOString()
  result.status = 'completed'

  onUpdate({ ...result })
  return result
}
