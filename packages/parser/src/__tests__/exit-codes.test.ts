import { describe, expect, it } from "vitest";
import {
  classifyExitCode,
  formatExitCode,
  getSignalFromExitCode,
  isConfigurationError,
  isSignalExit,
  isTransientFailure,
} from "../exit-codes.js";

describe("exit-codes", () => {
  describe("classifyExitCode", () => {
    describe("success", () => {
      it("classifies exit code 0 as success", () => {
        const result = classifyExitCode(0);
        expect(result.classification).toBe("success");
        expect(result.severity).toBe("info");
        expect(result.message).toBe("Success");
        expect(result.isTransient).toBe(false);
        expect(result.isConfiguration).toBe(false);
      });
    });

    describe("general errors", () => {
      it("classifies exit code 1 as general error", () => {
        const result = classifyExitCode(1);
        expect(result.classification).toBe("general");
        expect(result.severity).toBe("error");
        expect(result.message).toBe("General error");
        expect(result.hint).toBe("Check the command output for details");
        expect(result.isTransient).toBe(false);
        expect(result.isConfiguration).toBe(false);
      });

      it("classifies exit code 2 as shell misuse", () => {
        const result = classifyExitCode(2);
        expect(result.classification).toBe("general");
        expect(result.message).toBe("Misuse of shell command");
        expect(result.isConfiguration).toBe(true);
      });

      it("classifies exit code 128 as invalid exit argument", () => {
        const result = classifyExitCode(128);
        expect(result.classification).toBe("general");
        expect(result.message).toBe("Invalid exit argument");
      });

      it("classifies unknown exit codes as general error", () => {
        const result = classifyExitCode(42);
        expect(result.classification).toBe("general");
        expect(result.message).toBe("Command failed with exit code 42");
      });
    });

    describe("not found errors", () => {
      it("classifies exit code 127 as not_found with isConfiguration=true", () => {
        const result = classifyExitCode(127);
        expect(result.classification).toBe("not_found");
        expect(result.severity).toBe("error");
        expect(result.message).toBe("Command or script not found");
        expect(result.hint).toBe("Check PATH or package.json scripts");
        expect(result.isConfiguration).toBe(true);
        expect(result.isTransient).toBe(false);
      });
    });

    describe("permission errors", () => {
      it("classifies exit code 126 as permission error", () => {
        const result = classifyExitCode(126);
        expect(result.classification).toBe("permission");
        expect(result.severity).toBe("error");
        expect(result.message).toBe("Command not executable");
        expect(result.hint).toBe("Run chmod +x on the script file");
        expect(result.isConfiguration).toBe(true);
      });
    });

    describe("signal exits", () => {
      it("classifies exit code 137 as SIGKILL signal", () => {
        const result = classifyExitCode(137);
        expect(result.classification).toBe("signal");
        expect(result.signal).toBe(9);
        expect(result.signalName).toBe("SIGKILL");
        expect(result.message).toBe("Killed (SIGKILL)");
        expect(result.hint).toBe(
          "Process was forcefully terminated, possibly due to OOM"
        );
        expect(result.isTransient).toBe(true);
        expect(result.isConfiguration).toBe(false);
      });

      it("classifies exit code 130 as SIGINT signal", () => {
        const result = classifyExitCode(130);
        expect(result.classification).toBe("signal");
        expect(result.signal).toBe(2);
        expect(result.signalName).toBe("SIGINT");
        expect(result.message).toBe("Interrupted (SIGINT)");
        expect(result.severity).toBe("info");
        expect(result.isTransient).toBe(false);
      });

      it("classifies exit code 143 as SIGTERM signal", () => {
        const result = classifyExitCode(143);
        expect(result.classification).toBe("signal");
        expect(result.signal).toBe(15);
        expect(result.signalName).toBe("SIGTERM");
        expect(result.message).toBe("Terminated (SIGTERM)");
      });

      it("classifies exit code 139 as SIGSEGV signal", () => {
        const result = classifyExitCode(139);
        expect(result.classification).toBe("signal");
        expect(result.signal).toBe(11);
        expect(result.signalName).toBe("SIGSEGV");
        expect(result.message).toBe("Segmentation fault (SIGSEGV)");
        expect(result.isTransient).toBe(false);
      });

      it("classifies exit code 141 as SIGPIPE signal", () => {
        const result = classifyExitCode(141);
        expect(result.classification).toBe("signal");
        expect(result.signal).toBe(13);
        expect(result.signalName).toBe("SIGPIPE");
        expect(result.isTransient).toBe(true);
      });

      it("classifies unknown signal exit codes correctly", () => {
        const result = classifyExitCode(200);
        expect(result.classification).toBe("signal");
        expect(result.signal).toBe(72);
        expect(result.signalName).toBeUndefined();
        expect(result.message).toBe("Terminated by signal 72");
      });
    });
  });

  describe("isConfigurationError", () => {
    it("returns true for exit code 127 (command not found)", () => {
      expect(isConfigurationError(127)).toBe(true);
    });

    it("returns true for exit code 126 (permission denied)", () => {
      expect(isConfigurationError(126)).toBe(true);
    });

    it("returns true for exit code 2 (shell misuse)", () => {
      expect(isConfigurationError(2)).toBe(true);
    });

    it("returns false for exit code 0 (success)", () => {
      expect(isConfigurationError(0)).toBe(false);
    });

    it("returns false for exit code 1 (general error)", () => {
      expect(isConfigurationError(1)).toBe(false);
    });

    it("returns false for signal exits", () => {
      expect(isConfigurationError(137)).toBe(false);
      expect(isConfigurationError(130)).toBe(false);
    });
  });

  describe("isTransientFailure", () => {
    it("returns true for SIGKILL (137) - likely OOM", () => {
      expect(isTransientFailure(137)).toBe(true);
    });

    it("returns true for SIGPIPE (141)", () => {
      expect(isTransientFailure(141)).toBe(true);
    });

    it("returns false for SIGINT (130)", () => {
      expect(isTransientFailure(130)).toBe(false);
    });

    it("returns false for SIGTERM (143)", () => {
      expect(isTransientFailure(143)).toBe(false);
    });

    it("returns false for success (0)", () => {
      expect(isTransientFailure(0)).toBe(false);
    });

    it("returns false for general errors (1)", () => {
      expect(isTransientFailure(1)).toBe(false);
    });
  });

  describe("isSignalExit", () => {
    it("returns true for exit codes > 128 and <= 255", () => {
      expect(isSignalExit(129)).toBe(true);
      expect(isSignalExit(137)).toBe(true);
      expect(isSignalExit(255)).toBe(true);
    });

    it("returns false for exit codes <= 128", () => {
      expect(isSignalExit(0)).toBe(false);
      expect(isSignalExit(1)).toBe(false);
      expect(isSignalExit(127)).toBe(false);
      expect(isSignalExit(128)).toBe(false);
    });

    it("returns false for exit codes > 255", () => {
      expect(isSignalExit(256)).toBe(false);
      expect(isSignalExit(300)).toBe(false);
    });
  });

  describe("getSignalFromExitCode", () => {
    it("returns signal number for signal exits", () => {
      expect(getSignalFromExitCode(137)).toBe(9);
      expect(getSignalFromExitCode(130)).toBe(2);
      expect(getSignalFromExitCode(143)).toBe(15);
    });

    it("returns undefined for non-signal exits", () => {
      expect(getSignalFromExitCode(0)).toBeUndefined();
      expect(getSignalFromExitCode(1)).toBeUndefined();
      expect(getSignalFromExitCode(127)).toBeUndefined();
    });
  });

  describe("formatExitCode", () => {
    it("formats success exit code", () => {
      expect(formatExitCode(0)).toBe("exit 0");
    });

    it("formats general error exit code", () => {
      expect(formatExitCode(1)).toBe("exit 1 (error)");
    });

    it("formats not found exit code", () => {
      expect(formatExitCode(127)).toBe("exit 127 (not found)");
    });

    it("formats permission denied exit code", () => {
      expect(formatExitCode(126)).toBe("exit 126 (permission denied)");
    });

    it("formats signal exit codes with signal name", () => {
      expect(formatExitCode(137)).toBe("exit 137 (SIGKILL)");
      expect(formatExitCode(130)).toBe("exit 130 (SIGINT)");
      expect(formatExitCode(143)).toBe("exit 143 (SIGTERM)");
    });

    it("formats unknown signal exit codes", () => {
      expect(formatExitCode(200)).toBe("exit 200 (signal)");
    });

    it("formats arbitrary exit codes", () => {
      expect(formatExitCode(42)).toBe("exit 42 (error)");
    });
  });

  describe("edge cases", () => {
    it("handles boundary value 128", () => {
      const result = classifyExitCode(128);
      expect(result.classification).toBe("general");
      expect(result.message).toBe("Invalid exit argument");
    });

    it("handles boundary value 129 (SIGHUP)", () => {
      const result = classifyExitCode(129);
      expect(result.classification).toBe("signal");
      expect(result.signal).toBe(1);
      expect(result.signalName).toBe("SIGHUP");
    });

    it("handles boundary value 255", () => {
      const result = classifyExitCode(255);
      expect(result.classification).toBe("signal");
      expect(result.signal).toBe(127);
    });
  });
});
