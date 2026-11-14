// @vitest-environment happy-dom

import { mount } from '@vue/test-utils';
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';

beforeAll(() => {
  const probe = document.createElement('div');
  const nodeNameGetter = Object.getOwnPropertyDescriptor(window.Node.prototype, 'nodeName')?.get;
  if (nodeNameGetter && nodeNameGetter.call(probe) === '') {
    Object.defineProperty(window.Node.prototype, 'nodeName', {
      configurable: true,
      enumerable: false,
      get() {
        if (this.nodeType === 1) return (this as Element).tagName;
        if (this.nodeType === 3) return '#text';
        if (this.nodeType === 8) return '#comment';
        if (this.nodeType === 9) return '#document';
        if (this.nodeType === 11) return '#document-fragment';
        return '';
      }
    });
  }

  const elementRemove = window.Element.prototype.remove;
  Object.defineProperty(window.Element.prototype, 'remove', {
    configurable: true,
    writable: true,
    value(this: Element) {
      return elementRemove.call(this);
    }
  });

  probe.innerHTML = '<span><i></i></span><b></b>';
  const probeIter = document.createNodeIterator(probe, window.NodeFilter.SHOW_ELEMENT);
  probeIter.nextNode();
  const secondNode = probeIter.nextNode();
  secondNode?.parentNode?.removeChild(secondNode);
  if (probeIter.nextNode()?.nodeName !== 'B') {
    let docProto: Record<string, unknown> | null = Object.getPrototypeOf(document);
    while (docProto && !Object.getOwnPropertyDescriptor(docProto, 'createNodeIterator')) {
      docProto = Object.getPrototypeOf(docProto);
    }
    if (docProto) {
      docProto.createNodeIterator = function createLiveNodeIterator(rootNode: Node, whatToShow = -1): NodeIterator {
        const visited = new WeakSet<Node>();
        const accepts = (node: Node) => {
          if (whatToShow === -1 || whatToShow === 0xffffffff) return true;
          const bit = 1 << (node.nodeType - 1);
          return (whatToShow & bit) !== 0;
        };
        return {
          root: rootNode,
          referenceNode: rootNode,
          pointerBeforeReferenceNode: true,
          whatToShow,
          filter: null,
          detach() {},
          previousNode() {
            return null;
          },
          nextNode() {
            const liveNodes: Node[] = [];
            const walk = (node: Node) => {
              liveNodes.push(node);
              node.childNodes.forEach(child => walk(child));
            };
            walk(rootNode);
            for (const candidate of liveNodes) {
              if (visited.has(candidate)) continue;
              visited.add(candidate);
              if (accepts(candidate)) return candidate;
            }
            return null;
          }
        } as NodeIterator;
      };
    }
  }
});

vi.mock('@/locales', () => ({
  $t: (key: string) => {
    const dict: Record<string, string> = {
      'chat.copy': '复制',
      'chat.copied': '已复制',
      'chat.copyCode': '复制代码',
      'chat.codeLanguage': 'text'
    };
    return dict[key] || key;
  }
}));

const mounted: Array<ReturnType<typeof mount>> = [];

afterEach(() => {
  mounted.splice(0).forEach(wrapper => wrapper.unmount());
  vi.restoreAllMocks();
});

async function mountMarkdown(content: string, streaming = false) {
  const { default: MarkdownRenderer } = await import('./markdown-renderer.vue');
  const wrapper = mount(MarkdownRenderer, {
    props: { content, streaming }
  });
  mounted.push(wrapper);
  return wrapper;
}

describe('MarkdownRenderer terminal code blocks, tables, and sanitization', () => {
  it('renders language header, wraps GFM tables, and copies raw code via delegated click', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText }
    });

    const wrapper = await mountMarkdown(
      [
        '### 部署步骤',
        '',
        '```typescript',
        'const ok = a < b && c > d;',
        '```',
        '',
        '| 指标 | 数值 |',
        '| --- | --- |',
        '| 延迟 | 42ms |'
      ].join('\n'),
      true
    );

    expect(wrapper.get('.markdown-renderer').classes()).toContain('markdown-renderer--streaming');
    expect(wrapper.find('.md-code-block__lang').text()).toBe('typescript');
    expect(wrapper.find('.md-table-wrap table').exists()).toBe(true);

    const copyButton = wrapper.get('button[data-md-copy="true"]');
    expect(copyButton.text()).toBe('复制');
    await copyButton.trigger('click');

    expect(writeText).toHaveBeenCalledWith('const ok = a < b && c > d;');
    expect(copyButton.attributes('data-copied')).toBe('true');
    expect(copyButton.text()).toBe('已复制');
  });

  it('preserves DOMPurify XSS protection and forbids img/iframe/form/style tags and inline style attributes', async () => {
    const wrapper = await mountMarkdown(
      [
        '安全段落',
        '',
        '<script>window.__xss = true</script>',
        '<img src="x" onerror="alert(1)" />',
        '<iframe></iframe>',
        '<form action="/submit"></form>',
        '<div style="position:fixed;inset:0" onclick="alert(2)">inline</div>',
        '',
        '```js" onclick="alert(3)',
        '<script>alert(4)</script>',
        '```'
      ].join('\n')
    );

    const html = wrapper.html();
    expect(wrapper.find('script').exists()).toBe(false);
    expect(wrapper.find('img').exists()).toBe(false);
    expect(wrapper.find('iframe').exists()).toBe(false);
    expect(wrapper.find('form').exists()).toBe(false);
    expect(html).not.toContain('onerror=');
    expect(html).not.toContain('onclick=');
    expect(html).not.toContain('style=');
    expect(wrapper.find('.md-code-block__lang').text()).toBe('js');
    expect(wrapper.find('pre code').text()).toBe('<script>alert(4)</script>');
  });
});
