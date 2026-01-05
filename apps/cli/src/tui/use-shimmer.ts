import { create, type Model } from "@handleui/shimmer";
import { useEffect, useRef, useState } from "react";

interface UseShimmerProps {
  text: string;
  baseColor: string;
  isLoading: boolean;
}

export const useShimmer = ({
  text,
  baseColor,
  isLoading,
}: UseShimmerProps): string => {
  const [output, setOutput] = useState(text);
  const modelRef = useRef<Model | null>(null);

  useEffect(() => {
    if (!modelRef.current) {
      modelRef.current = create(text, baseColor, {
        interval: 40,
        peakLight: 100,
        waveWidth: 6,
      });
      modelRef.current.setOnTick(() => {
        if (modelRef.current) {
          setOutput(modelRef.current.view());
        }
      });
    }

    modelRef.current.setText(text);
    modelRef.current.setLoading(isLoading);

    if (isLoading) {
      modelRef.current.init();
    } else {
      modelRef.current.stop();
    }

    return () => {
      modelRef.current?.stop();
    };
  }, [text, baseColor, isLoading]);

  return isLoading ? output : text;
};
