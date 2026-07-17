<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { defaultKeymap, history, historyKeymap } from '@codemirror/commands';
import { markdown } from '@codemirror/lang-markdown';
import { defaultHighlightStyle, syntaxHighlighting } from '@codemirror/language';
import { Compartment, EditorState } from '@codemirror/state';
import { EditorView, keymap, lineNumbers, placeholder as cmPlaceholder } from '@codemirror/view';
import { useThemeStore } from '@/store/modules/theme';

defineOptions({ name: 'MarkdownPromptEditor' });

const props = withDefaults(
  defineProps<{
    modelValue?: string;
    placeholder?: string;
    maxlength?: number;
    disabled?: boolean;
  }>(),
  {
    modelValue: '',
    placeholder: '',
    maxlength: 0,
    disabled: false
  }
);

const emit = defineEmits<{
  'update:modelValue': [value: string];
}>();

const themeStore = useThemeStore();
const hostRef = ref<HTMLElement | null>(null);
const viewRef = ref<EditorView | null>(null);
const syncingFromProp = ref(false);

const themeCompartment = new Compartment();
const placeholderCompartment = new Compartment();
const editableCompartment = new Compartment();

const charCount = computed(() => (props.modelValue || '').length);
const showCount = computed(() => props.maxlength > 0);

function editorTheme(dark: boolean) {
  return EditorView.theme(
    {
      '&': {
        height: '100%',
        fontSize: '14px',
        borderRadius: '6px',
        border: `1px solid ${dark ? 'rgba(255,255,255,0.16)' : 'rgba(0,0,0,0.12)'}`,
        backgroundColor: dark ? 'rgba(255,255,255,0.04)' : '#fff'
      },
      '&.cm-focused': {
        outline: 'none',
        borderColor: 'var(--n-border-hover, #63e2b7)'
      },
      '.cm-scroller': {
        overflow: 'auto',
        fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
        lineHeight: '1.55'
      },
      '.cm-content': {
        padding: '10px 0',
        caretColor: dark ? '#e6edf3' : '#1f2328',
        minHeight: '100%'
      },
      '.cm-gutters': {
        backgroundColor: dark ? 'rgba(255,255,255,0.03)' : '#f6f8fa',
        color: dark ? '#8b949e' : '#656d76',
        borderRight: `1px solid ${dark ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.06)'}`,
        minHeight: '100%'
      },
      '.cm-activeLineGutter': {
        backgroundColor: dark ? 'rgba(255,255,255,0.06)' : 'rgba(0,0,0,0.04)'
      },
      '.cm-activeLine': {
        backgroundColor: dark ? 'rgba(255,255,255,0.04)' : 'rgba(0,0,0,0.03)'
      },
      '.cm-selectionBackground, &.cm-focused .cm-selectionBackground': {
        backgroundColor: dark ? 'rgba(56,139,253,0.35)' : 'rgba(9,105,218,0.2)'
      },
      '.cm-cursor': {
        borderLeftColor: dark ? '#e6edf3' : '#1f2328'
      },
      '.cm-placeholder': {
        color: dark ? 'rgba(255,255,255,0.38)' : 'rgba(0,0,0,0.38)'
      }
    },
    { dark }
  );
}

function createState(doc: string) {
  return EditorState.create({
    doc,
    extensions: [
      history(),
      lineNumbers(),
      EditorView.lineWrapping,
      keymap.of([...defaultKeymap, ...historyKeymap]),
      markdown(),
      syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
      themeCompartment.of(editorTheme(themeStore.darkMode)),
      placeholderCompartment.of(props.placeholder ? cmPlaceholder(props.placeholder) : []),
      editableCompartment.of(EditorView.editable.of(!props.disabled)),
      EditorState.transactionFilter.of(tr => {
        if (!tr.docChanged || !props.maxlength) return tr;
        if (tr.newDoc.length <= props.maxlength) return tr;
        return [];
      }),
      EditorView.updateListener.of(update => {
        if (!update.docChanged || syncingFromProp.value) return;
        emit('update:modelValue', update.state.doc.toString());
      })
    ]
  });
}

onMounted(() => {
  if (!hostRef.value) return;
  viewRef.value = new EditorView({
    state: createState(props.modelValue || ''),
    parent: hostRef.value
  });
});

onBeforeUnmount(() => {
  viewRef.value?.destroy();
  viewRef.value = null;
});

watch(
  () => props.modelValue,
  value => {
    const view = viewRef.value;
    if (!view) return;
    const next = value || '';
    if (next === view.state.doc.toString()) return;
    syncingFromProp.value = true;
    view.dispatch({
      changes: { from: 0, to: view.state.doc.length, insert: next }
    });
    syncingFromProp.value = false;
  }
);

watch(
  () => themeStore.darkMode,
  dark => {
    viewRef.value?.dispatch({
      effects: themeCompartment.reconfigure(editorTheme(dark))
    });
  }
);

watch(
  () => props.placeholder,
  text => {
    viewRef.value?.dispatch({
      effects: placeholderCompartment.reconfigure(text ? cmPlaceholder(text) : [])
    });
  }
);

watch(
  () => props.disabled,
  disabled => {
    viewRef.value?.dispatch({
      effects: editableCompartment.reconfigure(EditorView.editable.of(!disabled))
    });
  }
);
</script>

<template>
  <div class="markdown-prompt-editor flex min-h-0 flex-1 flex-col">
    <div ref="hostRef" class="markdown-prompt-editor__host min-h-0 flex-1 overflow-hidden" />
    <div v-if="showCount" class="mt-6px shrink-0 text-right text-12px text-gray-500">
      {{ charCount }} / {{ maxlength }}
    </div>
  </div>
</template>

<style scoped>
.markdown-prompt-editor__host {
  min-height: 0;
}

.markdown-prompt-editor__host :deep(.cm-editor) {
  height: 100%;
}

.markdown-prompt-editor__host :deep(.cm-editor.cm-focused) {
  outline: none;
}
</style>
