import { expect, test } from '@playwright/test';
import { SSEParser } from '../src/lib/sse';

const encoder = new TextEncoder();

function parseChunks(chunks: string[]) {
  const parser = new SSEParser();
  return chunks.flatMap((chunk) => parser.push(encoder.encode(chunk))).concat(parser.finish());
}

test('按 SSE 规则合并多行 data 并兼容 CRLF', () => {
  expect(parseChunks(['event: token\r\ndata: 第一行\r\ndata: 第二行\r\n\r\n'])).toEqual([
    { event: 'token', data: '第一行\n第二行' },
  ]);
});

test('接受 data 前缀后无空格的事件', () => {
  expect(parseChunks(['event: token\ndata:内容\n\n'])).toEqual([
    { event: 'token', data: '内容' },
  ]);
});

test('跨网络分块还原 UTF-8 文本', () => {
  expect(parseChunks(['event: token\ndata: 你', '好\n\n'])).toEqual([
    { event: 'token', data: '你好' },
  ]);
});

test('在 EOF 时刷新未以空行结尾的最后事件', () => {
  expect(parseChunks(['event: done\ndata: [DONE]'])).toEqual([
    { event: 'done', data: '[DONE]' },
  ]);
});
