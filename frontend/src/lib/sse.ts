export interface SSEEvent {
  event: string;
  data: string;
}

/**
 * 浏览器端增量 SSE 解析器：跨字节分块保留未完成事件，并按 SSE 规则合并 data 行。
 */
export class SSEParser {
  private readonly decoder = new TextDecoder();
  private buffer = '';

  push(chunk: Uint8Array): SSEEvent[] {
    return this.parse(this.decoder.decode(chunk, { stream: true }));
  }

  finish(): SSEEvent[] {
    const events = this.parse(this.decoder.decode());
    if (!this.buffer) {
      return events;
    }

    const tail = parseSSEEvent(this.buffer);
    this.buffer = '';
    return tail ? [...events, tail] : events;
  }

  private parse(text: string): SSEEvent[] {
    this.buffer += text;

    const eventBlocks = this.buffer.split(/\r?\n\r?\n/);
    this.buffer = eventBlocks.pop() ?? '';

    return eventBlocks.map(parseSSEEvent).filter((event): event is SSEEvent => event !== null);
  }
}

function parseSSEEvent(block: string): SSEEvent | null {
  let event = '';
  const dataLines: string[] = [];

  for (const line of block.split(/\r?\n/)) {
    if (line.startsWith('event:')) {
      event = removeOptionalSpace(line.slice(6));
    } else if (line.startsWith('data:')) {
      dataLines.push(removeOptionalSpace(line.slice(5)));
    }
  }

  return event || dataLines.length > 0 ? { event, data: dataLines.join('\n') } : null;
}

function removeOptionalSpace(value: string): string {
  return value.startsWith(' ') ? value.slice(1) : value;
}
