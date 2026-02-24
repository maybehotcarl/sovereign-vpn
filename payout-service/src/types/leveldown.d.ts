declare module "leveldown" {
  import type { AbstractLevelDOWN } from "abstract-leveldown";
  function LevelDOWN(location: string): AbstractLevelDOWN;
  export = LevelDOWN;
}
