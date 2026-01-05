import type { TUIEvent } from "../tui/check-tui-types.js";

/**
 * Simple type-safe event emitter for TUI events
 */
export class TUIEventEmitter {
  private listeners: Array<(event: TUIEvent) => void> = [];

  /**
   * Subscribe to events
   * @returns Unsubscribe function
   */
  on(callback: (event: TUIEvent) => void): () => void {
    this.listeners.push(callback);
    return () => {
      const index = this.listeners.indexOf(callback);
      if (index > -1) {
        this.listeners.splice(index, 1);
      }
    };
  }

  /**
   * Emit an event to all listeners
   */
  emit(event: TUIEvent): void {
    for (const listener of this.listeners) {
      listener(event);
    }
  }

  /**
   * Remove all listeners
   */
  removeAllListeners(): void {
    this.listeners = [];
  }
}
